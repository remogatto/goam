package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] COMMAND\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Command is one of:\n")
	fmt.Fprintf(os.Stderr, "    info\n")
	fmt.Fprintf(os.Stderr, "    make\n")
	fmt.Fprintf(os.Stderr, "    make-tests\n")
	fmt.Fprintf(os.Stderr, "    test [PATTERN]\n")
	fmt.Fprintf(os.Stderr, "    benchmark [PATTERN]\n")
	fmt.Fprintf(os.Stderr, "    clean\n")
	fmt.Fprintf(os.Stderr, "    install\n")
	fmt.Fprintf(os.Stderr, "    uninstall\n")
	fmt.Fprintf(os.Stderr, "    install-deps\n")
	fmt.Fprintf(os.Stderr, "    gofmt\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	flag.PrintDefaults()
}

func exitIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func boot(updateTests bool) (*dir_t, error) {
	rootObject, err := readDir()
	if err != nil {
		return nil, err
	}

	for len(newObjects) > 0 {
		objects := newObjects

		// Clean 'newObjects'
		newObjects = make(map[object_t]byte)

		for object := range objects {
			// Maybe infer some more objects.
			// All created objects are added to 'newObjects'.
			err = object.InferObjects(updateTests)
			if err != nil {
				return nil, err
			}
		}
	}

	err = check_executable2sources(rootObject)
	if err != nil {
		return nil, err
	}

	for _, remotePackage := range remotePackages {
		err = remotePackage.Check()
		if err != nil {
			return nil, err
		}
	}

	if *flag_debug {
		rootObject.PrintDependencies(os.Stdout)
	}

	return rootObject, nil
}

func info([]string) error {
	rootObject, err := boot( /*updateTests*/ false)
	if err != nil {
		return err
	}

	i := new_info()
	rootObject.Info(i)
	buf := bufio.NewWriter(os.Stdout)
	i.Print(buf)
	buf.Flush()

	return nil
}

func _make([]string) error {
	rootObject, err := boot( /*updateTests*/ false)
	if err != nil {
		return err
	}

	err = installAllRemotePackages()
	if err != nil {
		return err
	}

	err = rootObject.Make()
	if err != nil {
		return err
	}

	return nil
}

func makeTests([]string) error {
	rootObject, err := boot( /*updateTests*/ true)
	if err != nil {
		return err
	}

	err = installAllRemotePackages()
	if err != nil {
		return err
	}

	err = rootObject.MakeTests()
	if err != nil {
		return err
	}

	return nil
}

func runTestsAndBenchmarks(testPattern, benchPattern string) error {
	rootObject, err := boot( /*updateTests*/ true)
	if err != nil {
		return err
	}

	err = installAllRemotePackages()
	if err != nil {
		return err
	}

	err = rootObject.MakeTests()
	if err != nil {
		return err
	}

	var errorList []error
	rootObject.RunTests(testPattern, benchPattern, &errorList)
	if len(errorList) > 0 {
		for _, err = range errorList {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		return errors.New("some tests have failed")
	}

	return nil
}

func runTests(args []string) error {
	var pattern string
	switch len(args) {
	case 0:
		pattern = ""
	case 1:
		pattern = args[0]
	default:
		panic(fmt.Sprintf("invalid number of arguments: %d", len(args)))
	}

	return runTestsAndBenchmarks(pattern, "")
}

func runBenchmarks(args []string) error {
	var pattern string
	switch len(args) {
	case 0:
		pattern = ".*"
	case 1:
		pattern = args[0]
	default:
		panic(fmt.Sprintf("invalid number of arguments: %d", len(args)))
	}

	return runTestsAndBenchmarks("\"<no-tests>\"", pattern)
}

func clean([]string) error {
	rootObject, err := boot( /*updateTests*/ false)
	if err != nil {
		return err
	}

	err = rootObject.Clean()
	if err != nil {
		return err
	}

	return nil
}

func install([]string) error {
	rootObject, err := boot( /*updateTests*/ false)
	if err != nil {
		return err
	}

	if *flag_gcc {
		return errors.New("there is no support for installation when using gccgo")
	}
	if len(installationCommands) == 0 {
		return errors.New("nothing to install")
	}

	err = installAllRemotePackages()
	if err != nil {
		return err
	}

	for _, cmd := range installationCommands {
		err = cmd.Install(rootObject)
		if err != nil {
			return err
		}
	}

	return nil
}

func uninstall([]string) error {
	rootObject, err := boot( /*updateTests*/ false)
	if err != nil {
		return err
	}

	if len(installationCommands) == 0 {
		return errors.New("nothing to uninstall")
	}

	for _, cmd := range installationCommands {
		err = cmd.Uninstall(rootObject)
		if err != nil {
			return err
		}
	}

	return nil
}

func installDependencies([]string) error {
	_, err := boot( /*updateTests*/ false)
	if err != nil {
		return err
	}

	if len(remotePackages) == 0 {
		return errors.New("there are no remote packages")
	}

	return installAllRemotePackages()
}

func gofmt([]string) error {
	rootObject, err := readDir()
	if err != nil {
		return err
	}

	var files []string
	err = rootObject.GoFmt(&files)
	if err != nil {
		return err
	}

	goFmt(files)

	return nil
}

type function_info_t struct {
	fn      func([]string) error
	minArgs int
	maxArgs int
}

var functionTable = map[string]function_info_t{
	"info":         {info, 0, 0},
	"make":         {_make, 0, 0},
	"make-tests":   {makeTests, 0, 0},
	"test":         {runTests, 0, 1},
	"benchmark":    {runBenchmarks, 0, 1},
	"clean":        {clean, 0, 0},
	"install":      {install, 0, 0},
	"uninstall":    {uninstall, 0, 0},
	"install-deps": {installDependencies, 0, 0},
	"gofmt":        {gofmt, 0, 0},
}

var (
	flag_timings   = flag.Bool("t", false, "Print timings pertaining executed commands")
	flag_verbose   = flag.Bool("v", false, "Verbose")
	flag_debug     = flag.Bool("d", false, "Print debugging messages")
	flag_dashboard = flag.Bool("dashboard", true, "Report public packages at "+dashboardURL)
	flag_version   = flag.Bool("version", false, "Print version and exit")
	flag_gcc       = flag.Bool("gcc", false, "Use gccgo as the compiler and linker")
	flag_arch      = flag.String("conf-arch", runtime.GOARCH, "The value of GOARCH to use when interpreting GOAM.conf files")
	flag_os        = flag.String("conf-os", runtime.GOOS, "The value of GOOS to use when interpreting GOAM.conf files")
)

func main() {
	flag.Usage = func() { fmt.Fprintln(os.Stderr); usage() }
	flag.Parse()
	initArch()

	args := flag.Args()
	if *flag_version {
		fmt.Fprintf(os.Stderr, "GOAM version: %d\n", VERSION)
		os.Exit(0)
	}
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	functionName := args[0]
	if function, ok := functionTable[functionName]; ok {
		if (len(args)-1 < function.minArgs) || (len(args)-1 > function.maxArgs) {
			usage()
			os.Exit(1)
		}

		err := function.fn(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			if *flag_timings {
				printTimings(os.Stdout)
			}
			os.Exit(1)
		} else {
			if *flag_timings {
				printTimings(os.Stdout)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "invalid command: %s\n", functionName)
		fmt.Fprintf(os.Stderr, "\n")
		usage()
		os.Exit(1)
	}
}
