package main

import (
	"container/vector"
	"fmt"
	"io"
	"sort"
)

type info_t struct {
	// Set of libraries. (The values of the map have no meaning.)
	libs map[*library_t]byte

	// Set of executables. (The values of the map have no meaning.)
	executables map[*executable_t]byte

	// Set of executables. (The values of the map have no meaning.)
	tests map[*executable_t]byte
}

func new_info() *info_t {
	return &info_t{
		libs:        make(map[*library_t]byte),
		executables: make(map[*executable_t]byte),
		tests:       make(map[*executable_t]byte),
	}
}

func (info *info_t) Print(w io.Writer) {
	haveEmptyLine := true
	if *flag_debug {
		haveEmptyLine = false
	}

	if len(info.libs) > 0 {
		if !haveEmptyLine {
			fmt.Fprintf(w, "\n")
		}
		fmt.Fprintf(w, "Libraries:\n")

		libs_byPath := make(map[string]*library_t)
		paths := make([]string, len(info.libs))
		i := 0
		for lib, _ := range info.libs {
			libs_byPath[lib.path] = lib
			paths[i] = lib.path
			i++
		}

		sort.SortStrings(paths)

		for _, path := range paths {
			lib := libs_byPath[path]

			fmt.Fprintf(w, "    %s", path)
			if lib.makefile_orNil != nil {
				fmt.Fprintf(w, " (Makefile)")
			}
			fmt.Fprintf(w, "\n")
			haveEmptyLine = false

			if *flag_verbose {
				var sources vector.StringVector

				var unit *compilation_unit_t
				for _, unit = range lib.sources {
					for _, src := range unit.sources {
						sources.Push(src.Path())
					}
				}

				if lib.makefile_orNil != nil {
					for _, src := range lib.makefile_orNil.sources {
						sources.Push(src.Path())
					}
				}

				sortAndPrintNames(w, "        ", sources)
				fmt.Fprintf(w, "\n")
				haveEmptyLine = true
			}
		}
	}

	printExecutables(w, "Executables", /*allowVerbose*/ true, info.executables, &haveEmptyLine)
	printExecutables(w, "Tests", /*allowVerbose*/ false, info.tests, &haveEmptyLine)

	if len(remotePackages) > 0 {
		if !haveEmptyLine {
			fmt.Fprintf(w, "\n")
		}
		fmt.Fprintf(w, "Remote dependencies:\n")

		for _, remotePkg := range remotePackages {
			fmt.Fprintf(w, "    %s  @%s\n", remotePkg.repository.Path(), remotePkg.repository.KindString())
			haveEmptyLine = false

			if *flag_verbose {
				importPaths := make([]string, len(remotePkg.importPaths))
				copy(importPaths, remotePkg.importPaths)

				sortAndPrintNames(w, "        ", importPaths)
				fmt.Fprintf(w, "\n")
				haveEmptyLine = true
			}
		}
	}

	if !haveEmptyLine && *flag_timings {
		fmt.Fprintf(w, "\n")
	}
}

func sortAndPrintNames(w io.Writer, prefix string, names []string) {
	sort.SortStrings(names)
	for _, name := range names {
		fmt.Fprintf(w, "%s%s\n", prefix, name)
	}
}

func printExecutables(w io.Writer, tag string, allowVerbose bool, executables map[*executable_t]byte, haveEmptyLine *bool) {
	if len(executables) > 0 {
		if !(*haveEmptyLine) {
			fmt.Fprintf(w, "\n")
		}
		fmt.Fprintf(w, "%s:\n", tag)

		exes_byPath := make(map[string]*executable_t)
		paths := make([]string, len(executables))
		i := 0
		for exe, _ := range executables {
			exes_byPath[exe.path] = exe
			paths[i] = exe.path
			i++
		}

		sort.SortStrings(paths)

		for _, path := range paths {
			exe := exes_byPath[path]

			fmt.Fprintf(w, "    %s", path)
			if exe.makefile_orNil != nil {
				fmt.Fprintf(w, " (Makefile)")
			}
			fmt.Fprintf(w, "\n")
			*haveEmptyLine = false

			if *flag_verbose && allowVerbose {
				var sources vector.StringVector

				var unit *compilation_unit_t
				for _, unit = range exe.sources {
					for _, src := range unit.sources {
						sources.Push(src.Path())
					}
				}

				if exe.makefile_orNil != nil {
					for _, src := range exe.makefile_orNil.sources {
						sources.Push(src.Path())
					}
				}

				sortAndPrintNames(w, "        ", sources)
				fmt.Fprintf(w, "\n")
				*haveEmptyLine = true
			}
		}
	}
}
