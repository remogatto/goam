package main

import (
	"os"
)

var gofmt_exe = &Executable{
	name: "gofmt",
}

func goFmt(path string) os.Error {
	if *flag_debug {
		println("gofmt:", path)
	}

	var args []string
	if *flag_verbose {
		args = []string{gofmt_exe.name, "-l", "-w", path}
	} else {
		args = []string{gofmt_exe.name, "-w", path}
	}

	return gofmt_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
}
