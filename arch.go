package main

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
)

// The extension of files created by the Go compiler (.5, .6, .8)
var o_ext string

// The Go compiler (5g, 6g, 8g, gccgo)
var goCompiler_name string
var goCompiler_flags []string

// The Go archiver (gopack, ar)
var goArchiver_name string
var goArchiver_flags []string
var goArchiver_libNamePrefix string

// The Go linker (5l, 6l, 8l, gccgo)
var goLinker_name string

// The directory where to put/find installed libraries
var libInstallRoot string = path.Join(runtime.GOROOT(), "pkg", runtime.GOOS+"_"+runtime.GOARCH)

// The directory where to put remote packages
var remotePkgInstallRoot string = path.Join(runtime.GOROOT(), "src", "pkg")

// The directory where to put executables
var exeInstallDir string

var goCompiler_exe *Executable = nil
var goArchiver_exe *Executable = nil
var goLinker_exe *Executable = nil

func initArch() {
	if !(*flag_gcc) {
		goArchiver_name = "gopack"
		goArchiver_flags = []string{"grc"}
		goArchiver_libNamePrefix = ""

		switch runtime.GOARCH {
		case "386":
			o_ext = ".8"
			goCompiler_name = "8g"
			goLinker_name = "8l"

		case "amd64":
			o_ext = ".6"
			goCompiler_name = "6g"
			goLinker_name = "6l"

		case "arm":
			o_ext = ".5"
			goCompiler_name = "5g"
			goLinker_name = "5l"

		default:
			fmt.Fprintf(os.Stderr, "unknown GOARCH: %s\n", runtime.GOARCH)
			os.Exit(1)
		}
	} else {
		o_ext = ".o"
		goCompiler_name = "gccgo"
		goCompiler_flags = []string{"-c"}
		goArchiver_name = "ar"
		goArchiver_flags = []string{"rc"}
		goArchiver_libNamePrefix = "lib"
		goLinker_name = "gccgo"
	}

	exeInstallDir = os.Getenv("GOBIN")
	if len(exeInstallDir) == 0 {
		exeInstallDir = path.Join(runtime.GOROOT(), "bin")
	}

	goCompiler_exe = &Executable{name: goCompiler_name}
	goArchiver_exe = &Executable{name: goArchiver_name}
	goLinker_exe = &Executable{name: goLinker_name}
}


var goCompilerVersion *uint = nil

// Requires Go release.2010-12-15.1
const min_compiler_version_for_cgo = 6980

func getGoCompilerVersion() (uint, os.Error) {
	if goCompilerVersion == nil {
		args := []string{goCompiler_exe.name, "-V"}
		stdout, _, err := goCompiler_exe.run(args, /*dir*/ "", /*in*/ "", /*mergeStdoutAndStderr*/ true)
		if err != nil {
			return 0, os.NewError("failed to determine Go compiler version: " + err.String())
		}

		stdout = strings.TrimSpace(stdout)
		var stdout_split []string = strings.Split(stdout, " ")
		if len(stdout_split) < 3 {
			return 0, os.NewError("failed to extract [Go compiler version] from string \"" + stdout + "\"" +
				" (possible cause: you didn't have the Mercurial versioning system installed when you were compiling the Go distribution)")
		}

		version, err := strconv.Atoui(strings.TrimRight(stdout_split[2], "+"))
		if (err != nil) && (len(stdout_split) >= 4) {
			version, err = strconv.Atoui(strings.TrimRight(stdout_split[3], "+"))
		}
		if err != nil {
			return 0, os.NewError("failed to extract [Go compiler version] from string \"" + stdout + "\"")
		}

		goCompilerVersion = new(uint)
		*goCompilerVersion = version
	}

	return *goCompilerVersion, nil
}
