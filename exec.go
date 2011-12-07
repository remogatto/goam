package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"syscall"
)

type TaskResourceUsage struct {
	// User time, in nanoseconds
	userTime int64

	// System time, in nanoseconds
	systemTime int64

	// The number of times the command was executed
	numInvocations int

	dontPrintAverage bool
}

func (t *TaskResourceUsage) TotalTime() int64 {
	return t.userTime + t.systemTime
}

func (t *TaskResourceUsage) AverageTime() float64 {
	return float64(t.TotalTime()) / float64(t.numInvocations)
}

var taskStats = make(map[string]*TaskResourceUsage)

func addResourceUsage(taskName string, usage *syscall.Rusage) {
	taskRUsage := taskStats[taskName]
	if taskRUsage == nil {
		taskRUsage = &TaskResourceUsage{
			userTime:       0,
			systemTime:     0,
			numInvocations: 0,
		}

		taskStats[taskName] = taskRUsage
	}

	taskRUsage.systemTime += int64(usage.Utime.Sec)*1e9 + int64(usage.Utime.Usec)*1e3
	taskRUsage.userTime += int64(usage.Stime.Sec)*1e9 + int64(usage.Stime.Usec)*1e3
	taskRUsage.numInvocations += 1
}

func addSelf() {
	// Remove any previous stats
	delete(taskStats, "(self)")
	delete(taskStats, "(self.gc)")

	// Enclose the following 5 lines in a comment
	// if the compiler is unable to find 'syscall.Getrusage'
	var usage syscall.Rusage
	errno := syscall.Getrusage(0, &usage)
	if errno == nil {
		addResourceUsage("(self)", &usage)
	}

	taskStats["(self.gc)"] = &TaskResourceUsage{
		userTime:         int64(runtime.MemStats.PauseTotalNs),
		systemTime:       0,
		numInvocations:   int(runtime.MemStats.NumGC),
		dontPrintAverage: true,
	}
}

func printTimings(out *os.File) {
	addSelf()

	sortedNames := make([]string, len(taskStats))
	maxNameLength := 0
	{
		i := 0
		for name := range taskStats {
			sortedNames[i] = name
			if len(name) > maxNameLength {
				maxNameLength = len(name)
			}
			i++
		}

		sort.StringSlice(sortedNames).Sort()
	}

	fmt.Fprintf(out, "Run times:\n")
	for _, name := range sortedNames {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "    %s:", name)
		for i := len(name); i < maxNameLength; i++ {
			buf.WriteString(" ")
		}
		taskRUsage := taskStats[name]
		fmt.Fprintf(&buf, " %.3f secs", float64(taskRUsage.TotalTime())/1e9)
		if taskRUsage.numInvocations >= 2 {
			if !taskRUsage.dontPrintAverage {
				fmt.Fprintf(&buf, " (%d invocations, %.3f secs per invocation)",
					taskRUsage.numInvocations, taskRUsage.AverageTime()/1e9)
			} else {
				fmt.Fprintf(&buf, " (%d invocations)", taskRUsage.numInvocations)
			}
		}

		fmt.Fprintf(out, "%s\n", buf.String())
	}
}

type Executable struct {
	name     string
	noLookup bool
	fullPath string // Cached path obtained by calling 'exec.LookPath(name)'
}

type RunFlags struct {
	stdin, stdout, stderr *os.File
	dontPrintCmd          bool
}

func (e *Executable) runSimply(argv []string, dir string, dontPrintCmd bool) error {
	flags := RunFlags{
		stdin:        os.Stdin,
		stdout:       os.Stdout,
		stderr:       os.Stderr,
		dontPrintCmd: false,
	}

	return e.run_lowLevel(argv, dir, flags)
}

// Runs 'e' as separate process, waits until it finishes,
// and returns the data the process sent to its output(s).
// The argument 'in' comprises the command's input.
func (e *Executable) run(argv []string, dir string, in string, mergeStdoutAndStderr bool) (stdout string, stderr string, err error) {
	stdin_r, stdin_w, err := os.Pipe()
	if err != nil {
		return "", "", err
	}

	stdout_r, stdout_w, err := os.Pipe()
	if err != nil {
		stdin_r.Close()
		stdin_w.Close()
		return "", "", err
	}

	var stderr_r, stderr_w *os.File
	if mergeStdoutAndStderr {
		stderr_r, stderr_w = stdout_r, stdout_w
	} else {
		stderr_r, stderr_w, err = os.Pipe()
		if err != nil {
			stdin_r.Close()
			stdin_w.Close()
			stdout_r.Close()
			stdout_w.Close()
			return "", "", err
		}
	}

	errChan := make(chan error, 3)
	stdoutChan := make(chan []byte, 1)
	stderrChan := make(chan []byte, 1)
	{
		// A goroutine for feeding STDIN
		go func() {
			_, err := stdin_w.WriteString(in)
			if err != nil {
				stdin_w.Close()
			} else {
				err = stdin_w.Close()
			}
			errChan <- err
		}()

		// A goroutine for consuming STDOUT.
		// Note: STDOUT is optionally merged with STDERR.
		go func() {
			stdout, err := ioutil.ReadAll(stdout_r)
			if err != nil {
				stdout_r.Close()
				errChan <- err
			} else {
				err = stdout_r.Close()
				errChan <- err
				stdoutChan <- stdout
			}
		}()

		if !mergeStdoutAndStderr {
			// A goroutine for consuming STDERR
			go func() {
				stderr, err := ioutil.ReadAll(stderr_r)
				if err != nil {
					stderr_r.Close()
					errChan <- err
				} else {
					err = stderr_r.Close()
					errChan <- err
					stderrChan <- stderr
				}
			}()
		} else {
			errChan <- nil
			stderrChan <- make([]byte, 0)
		}
	}

	// Execute the command
	flags := RunFlags{
		stdin:        stdin_r,
		stdout:       stdout_w,
		stderr:       stderr_w,
		dontPrintCmd: true,
	}
	err = e.run_lowLevel(argv, dir, flags)

	// Close our ends of the pipes, so that the I/O goroutines can finish
	stdin_r.Close()
	stdout_w.Close()
	if stderr_w != stdout_w {
		stderr_w.Close()
	}

	if err != nil {
		return "", "", err
	}

	err1 := <-errChan
	err2 := <-errChan
	err3 := <-errChan
	if (err1 != nil) || (err2 != nil) || (err3 != nil) {
		err = err1
		if err == nil {
			err = err2
		}
		if err == nil {
			err = err3
		}
		return "", "", err
	}

	stdout = string(<-stdoutChan)
	stderr = string(<-stderrChan)
	return stdout, stderr, nil
}

// Runs 'e' as a separate process and waits until it finishes
func (e *Executable) run_lowLevel(argv []string, dir string, flags RunFlags) error {
	// Resolve 'e.fullpath' (if not resolved yet)
	if len(e.fullPath) == 0 {
		if (e.noLookup == false) || !strings.HasPrefix(e.name, "./") {
			var err error
			e.fullPath, err = exec.LookPath(e.name)
			if err != nil {
				msg := "failed to lookup executable \"" + e.name + "\": " + err.Error()
				return errors.New(msg)
			}
		} else {
			e.fullPath = e.name
		}
	}

	if dir == "." {
		dir = ""
	}

	if (*flag_verbose && !flags.dontPrintCmd) || *flag_debug {
		if len(dir) == 0 {
			fmt.Fprintf(os.Stdout, "(%s)\n", strings.Join(argv, " "))
		} else {
			fmt.Fprintf(os.Stdout, "(cd %s ; %s)\n", dir, strings.Join(argv, " "))
		}
	}

	procAttr := os.ProcAttr{
		Dir:   dir,
		Files: []*os.File{flags.stdin, flags.stdout, flags.stderr},
	}
	process, err := os.StartProcess(e.fullPath, argv, &procAttr)
	if err != nil {
		return err
	}

	waitMsg, err := process.Wait( /*options*/ os.WRUSAGE)
	if err != nil {
		return err
	}
	if *flag_timings {
		addResourceUsage(e.name, waitMsg.Rusage)
	}
	if !waitMsg.Exited() {
		return errors.New(fmt.Sprintf("unable to obtain the exit status of \"%s\"", e.name))
	}
	if waitMsg.ExitStatus() != 0 {
		var errMsg string
		if len(dir) == 0 {
			errMsg = fmt.Sprintf("command \"%s\" returned an error", strings.Join(argv, " "))
		} else {
			errMsg = fmt.Sprintf("command \"%s\" run in directory \"%s\" returned an error", strings.Join(argv, " "), dir)
		}
		return errors.New(errMsg)
	}

	return nil
}
