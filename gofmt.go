package main

import "fmt"

var gofmt_exe = &Executable{
	name: "gofmt",
}

func goFmt(paths []string) error {
	if *flag_debug {
		fmt.Println("gofmt: %v", paths)
	}

	if len(paths) == 0 {
		return nil
	}

	var args []string
	if *flag_verbose {
		args = []string{gofmt_exe.name, "-l", "-w"}
	} else {
		args = []string{gofmt_exe.name, "-w"}
	}

	args = append(args, paths...)

	return gofmt_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
}
