package main

import (
	"bytes"
	"exec"
	"fmt"
	"io/ioutil"
	"os"
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
	taskStats["(self)"] = nil, false
	taskStats["(self.gc)"] = nil, false

	var usage syscall.Rusage
	errno := syscall.Getrusage(0, &usage)
	if errno == 0 {
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
		for name, _ := range taskStats {
			sortedNames[i] = name
			if len(name) > maxNameLength {
				maxNameLength = len(name)
			}
			i++
		}

		sort.StringArray(sortedNames).Sort()
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
	cmdChan_orNil         chan *exec.Cmd
	stdin, stdout, stderr int
	dontPrintCmd          bool
}

func (e *Executable) runSimply(argv []string, dir string, dontPrintCmd bool) os.Error {
	flags := RunFlags{
		cmdChan_orNil: nil,
		stdin:         exec.PassThrough,
		stdout:        exec.PassThrough,
		stderr:        exec.PassThrough,
		dontPrintCmd:  false,
	}

	return e.run_lowLevel(argv, dir, flags)
}

// Runs 'e' as separate process, waits until it finishes,
// and returns the data the process sent to its output(s).
// The argument 'in' comprises the command's input.
func (e *Executable) run(argv []string, dir string, in string, mergeStdoutAndStderr bool) (stdout string, stderr string, err os.Error) {
	cmdChan := make(chan *exec.Cmd)
	errChan := make(chan os.Error, 3)
	stdoutChan := make(chan []byte, 1)
	stderrChan := make(chan []byte, 1)
	go func() {
		cmd := <-cmdChan
		if cmd != nil {
			// A goroutine for feeding STDIN
			go func() {
				_, err := cmd.Stdin.WriteString(in)
				if err != nil {
					cmd.Stdin.Close()
				} else {
					err = cmd.Stdin.Close()
				}
				errChan <- err
			}()

			// A goroutine for consuming STDOUT.
			// Note: STDOUT is optionally merged with STDERR.
			go func() {
				stdout, err := ioutil.ReadAll(cmd.Stdout)
				if err != nil {
					cmd.Stdout.Close()
					errChan <- err
				} else {
					err = cmd.Stdout.Close()
					errChan <- err
					stdoutChan <- stdout
				}
			}()

			if cmd.Stderr != nil {
				// A goroutine for consuming STDERR
				go func() {
					stderr, err := ioutil.ReadAll(cmd.Stderr)
					if err != nil {
						cmd.Stderr.Close()
						errChan <- err
					} else {
						err = cmd.Stderr.Close()
						errChan <- err
						stderrChan <- stderr
					}
				}()
			} else {
				errChan <- nil
				stderrChan <- make([]byte, 0)
			}
		}
	}()

	var stderrHandling int
	if mergeStdoutAndStderr {
		stderrHandling = exec.MergeWithStdout
	} else {
		stderrHandling = exec.Pipe
	}

	flags := RunFlags{
		cmdChan_orNil: cmdChan,
		stdin:         exec.Pipe,
		stdout:        exec.Pipe,
		stderr:        stderrHandling,
		dontPrintCmd:  true,
	}

	err = e.run_lowLevel(argv, dir, flags)
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

// Runs 'e' as separate process and waits until it finishes.
// If 'cmdChan_orNil' is not nil, it will receive the 'exec.Cmd' returned by 'exec.Run'.
func (e *Executable) run_lowLevel(argv []string, dir string, flags RunFlags) os.Error {
	// Resolve 'e.fullpath' (if not resolved yet)
	if len(e.fullPath) == 0 {
		if (e.noLookup == false) || !strings.HasPrefix(e.name, "./") {
			var err os.Error
			e.fullPath, err = exec.LookPath(e.name)
			if err != nil {
				msg := "failed to lookup executable \"" + e.name + "\": " + err.String()
				return os.NewError(msg)
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

	cmd, err := exec.Run(e.fullPath, argv, os.Environ(), dir, flags.stdin, flags.stdout, flags.stderr)
	if err != nil {
		if flags.cmdChan_orNil != nil {
			flags.cmdChan_orNil <- nil
		}
		return err
	} else {
		if flags.cmdChan_orNil != nil {
			flags.cmdChan_orNil <- cmd
		}
	}

	waitMsg, err := cmd.Wait( /*options*/ os.WRUSAGE)
	if err != nil {
		return err
	}
	if *flag_timings {
		addResourceUsage(e.name, waitMsg.Rusage)
	}
	if !waitMsg.Exited() {
		return os.NewError("unable to obtain the exit status of \"" + e.name + "%s\"")
	}
	if waitMsg.ExitStatus() != 0 {
		var errMsg string
		if len(dir) == 0 {
			errMsg = fmt.Sprintf("command \"%s\" returned an error", strings.Join(argv, " "))
		} else {
			errMsg = fmt.Sprintf("command \"%s\" run in directory \"%s\" returned an error", strings.Join(argv, " "), dir)
		}
		return os.NewError(errMsg)
	}

	return nil
}
