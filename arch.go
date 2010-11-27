package main

import (
	"fmt"
	"os"
	"path"
	"runtime"
)

// The extension of files created by the Go compiler (.5, .6, .8)
var o_ext string

// The Go compiler (5g, 6g, 8g)
var goCompilerName string

// The Go linker (5l, 6l, 8l)
var goLinkerName string

// The directory where to put/find installed libraries
var libInstallRoot string = path.Join(runtime.GOROOT(), "pkg", runtime.GOOS+"_"+runtime.GOARCH)

// The directory where to put remote packages
var remotePkgInstallRoot string = path.Join(runtime.GOROOT(), "src", "pkg")

// The directory where to put executables
var exeInstallDir string

var goCompiler_exe *Executable = nil
var goArchiver_exe *Executable = nil
var goLinker_exe *Executable = nil

func init() {
	switch runtime.GOARCH {
	case "386":
		o_ext = ".8"
		goCompilerName = "8g"
		goLinkerName = "8l"

	case "amd64":
		o_ext = ".6"
		goCompilerName = "6g"
		goLinkerName = "6l"

	case "arm":
		o_ext = ".5"
		goCompilerName = "5g"
		goLinkerName = "5l"

	default:
		fmt.Fprintf(os.Stderr, "unknown GOARCH: %s\n", runtime.GOARCH)
		os.Exit(1)
	}

	exeInstallDir = os.Getenv("GOBIN")
	if len(exeInstallDir) == 0 {
		exeInstallDir = path.Join(runtime.GOROOT(), "bin")
	}

	goCompiler_exe = &Executable{name: goCompilerName}
	goArchiver_exe = &Executable{name: "gopack"}
	goLinker_exe = &Executable{name: goLinkerName}
}
