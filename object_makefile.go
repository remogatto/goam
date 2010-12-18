package main

import (
	"container/vector"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	pathutil "path"
	"strings"
)

// Represents a Makefile
type makefile_t struct {
	entry_t
	parent      *dir_t
	contents    *makefile_contents_t // Initially nil
	sources     []go_source_code_t
	built       bool
	nowBuilding bool
}

const (
	MAKEFILE_CMD = iota
	MAKEFILE_PKG
	MAKEFILE_UNKNOWN_TYPE
)

type makefile_contents_t struct {
	targ    string
	kind    int // One of: MAKEFILE_CMD, MAKEFILE_PKG, MAKEFILE_UNKNOWN_TYPE
	goFiles []string
	cgo     bool
}


// ==========
// makefile_t
// ==========

func new_makefile(entry entry_t, parent *dir_t) (*makefile_t, os.Error) {
	makefile := &makefile_t{
		entry_t: entry,
		parent:  parent,
		sources: make([]go_source_code_t, 0, 0),
	}

	if parent.makefile_orNil == nil {
		parent.makefile_orNil = makefile
	} else {
		return nil, os.NewError("directory \"" + parent.path + "\" contains multiple makefiles")
	}

	newObjects[makefile] = 0
	return makefile, nil
}

func (m *makefile_t) UpdateFileSystemModel() {
	m.UpdateFileInfo()
}

func (m *makefile_t) Contents() (*makefile_contents_t, os.Error) {
	if m.contents == nil {
		var err os.Error
		m.contents, err = read_makefile_contents(m.path, m.parent.path)
		if err != nil {
			return nil, err
		}
	}

	return m.contents, nil
}

func (m *makefile_t) InferObjects(updateTests bool) os.Error {
	if !m.exists {
		return nil
	}

	contents, err := m.Contents()
	if err != nil {
		return err
	}

	if len(contents.targ) == 0 {
		return nil
	}

	if len(contents.goFiles) > 0 {
		sources := make([]go_source_code_t, len(contents.goFiles))

		for i, goFile := range contents.goFiles {
			goFile = pathutil.Clean(goFile)

			if !strings.HasSuffix(goFile, ".go") {
				return os.NewError("makefile \"" + m.Path() + "\": Go file \"" + goFile + "\"' should have .go extension")
			}

			var _src object_t = m.parent.getObject_orNil(strings.Split(goFile, "/", -1))

			var src go_source_code_t
			if _src == nil {
				return os.NewError("failed to find file \"" + goFile + "\"")
			} else {
				var isSrc bool
				src, isSrc = _src.(go_source_code_t)
				if !isSrc {
					return os.NewError("file \"" + _src.Path() + "\" was expected to be a Go file")
				}
				if !src.Exists() {
					return os.NewError("file \"" + _src.Path() + "\" does not exist")
				}
			}

			sources[i] = src
		}

		m.sources = sources
	}

	if contents.kind == MAKEFILE_CMD {
		var exe *executable_t

		exe_dir := m.parent
		exe_name := pathutil.Base(contents.targ)
		exe, err := exe_dir.getOrCreate_executable(exe_name)
		if err != nil {
			return err
		}

		// Register 'm' with the 'exe'
		err = exe.addMakefile(m)
		if err != nil {
			return err
		}
	} else if contents.kind == MAKEFILE_PKG {
		objDir := m.parent.getOrCreateSubDir("_obj")

		dirPath, baseName := pathutil.Split(contents.targ)
		lib_dir := objDir.getOrCreateSubDirs(strings.Split(dirPath, "/", -1))
		lib_name := baseName + ".a"

		var lib *library_t
		lib, err = lib_dir.getOrCreate_library(lib_name)
		if err != nil {
			return err
		}

		// Register 'm' with the 'lib'
		err = lib.addMakefile(m)
		if err != nil {
			return err
		}

		// Add 'lib' to the package resolution table
		err = mapImportPath(contents.targ, lib, objDir, /*test*/ false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *makefile_t) PrintDependencies(w io.Writer) {
	return
}

func (m *makefile_t) Info(info *info_t) {
	return
}

var make_exe = &Executable{
	name: "make",
}

func (m *makefile_t) Make() os.Error {
	if m.built {
		return nil
	}

	if m.nowBuilding {
		return os.NewError("circular dependency involving \"" + m.path + "\"")
	}
	m.nowBuilding = true
	defer func() { m.nowBuilding = false }()

	// Build all prerequisites
	{
		for _, src := range m.sources {
			contents, err := src.Contents()
			if err != nil {
				return err
			}

			_, err = contents.makePrerequisites( /*testPackage_orEmpty*/ "")
			if err != nil {
				return err
			}
		}
	}

	args := []string{make_exe.name, "-f", m.name}

	err := make_exe.runSimply(args, /*dir*/ m.parent.path, /*dontPrint*/ false)
	if err != nil {
		return err
	}

	// Update our internal model of filesystem contents
	m.parent.UpdateFileSystemModel()

	m.built = true
	return nil
}

func (m *makefile_t) MakeInstall() os.Error {
	// Build all prerequisites and build the target
	err := m.Make()
	if err != nil {
		return err
	}

	args := []string{make_exe.name, "-f", m.name, "install"}
	err = make_exe.runSimply(args, /*dir*/ m.parent.path, /*dontPrint*/ false)
	if err != nil {
		return err
	}

	// Update our internal model of filesystem contents
	m.parent.UpdateFileSystemModel()

	return nil
}

func (m *makefile_t) MakeTests() os.Error {
	args := []string{make_exe.name, "-f", m.name, "test"}
	err := make_exe.runSimply(args, /*dir*/ m.parent.path, /*dontPrint*/ false)
	if err != nil {
		return err
	}

	// Update our internal model of filesystem contents
	m.parent.UpdateFileSystemModel()

	return nil
}

func (m *makefile_t) RunTests(testPattern, benchPattern string) os.Error {
	return m.MakeTests()
}

func (m *makefile_t) Clean() os.Error {
	args := []string{make_exe.name, "-f", m.name, "clean"}
	err := make_exe.runSimply(args, /*dir*/ m.parent.path, /*dontPrint*/ false)
	if err != nil {
		return err
	}

	// Update our internal model of filesystem contents
	m.parent.UpdateFileSystemModel()

	return nil
}

func (m *makefile_t) GoFmt(files *vector.StringVector) os.Error {
	return nil
}


// ===================
// makefile_contents_t
// ===================

const make_print_ruleName = "__printMakefileVars"
const make_print_rule = "\n\n" + make_print_ruleName + ": \n" +
	"\t@echo $(TARG)\n" +
	"\t@echo $(GOFILES)\n" +
	"\t@echo $(CGOFILES)\n"

func read_makefile_contents(path, dir string) (*makefile_contents_t, os.Error) {
	var makefileText string
	{
		_makefileText, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}

		makefileText = string(_makefileText)
	}

	var makeOutput_lines []string
	{
		// Execute "make -f -"
		args := []string{make_exe.name, "-f", "-", make_print_ruleName}
		makeOutput, _, err := make_exe.run(args, dir, (makefileText + make_print_rule), /*mergeStdoutAndStderr*/ false)
		if err != nil {
			return nil, os.NewError("failed to extract needed variables from \"" + path + "\": " + err.String())
		}

		makeOutput_lines = strings.Split(makeOutput, "\n", -1)
		if len(makeOutput_lines) < 3 {
			return nil, os.NewError("failed to extract needed variables from \"" + path + "\"")
		}
	}

	var targ string = strings.TrimSpace(makeOutput_lines[0])
	var goFiles []string = strings.Fields(makeOutput_lines[1])
	var cgoFiles string = strings.TrimSpace(makeOutput_lines[2])

	if len(targ) == 0 {
		goFiles = make([]string, 0)
		cgoFiles = ""
	}

	if len(cgoFiles) > 0 {
		// The variable GOFILES was modified by
		// the (pressumably) included "$(GOROOT)/src/Make.{cmd,pkg,...}".
		// Reconstruct the original value of GOFILES:

		i, j := 0, 0
		for i < len(goFiles) {
			goFiles_i := goFiles[i]
			if strings.HasPrefix(goFiles_i, "_cgo_") {
				// Ignore 'goFiles[i]'
				i++
			} else if strings.HasSuffix(goFiles_i, ".cgo1.go") {
				f := goFiles_i
				goFiles[j] = f[0:len(f)-len(".cgo1.go")] + ".go"
				i++
				j++
			} else {
				goFiles[j] = goFiles_i
				i++
				j++
			}
		}
		goFiles = goFiles[0:j]
	}

	if *flag_debug {
		fmt.Printf("read makefile: %s\n", path)
		fmt.Printf("    TARG:    %s\n", targ)
		fmt.Printf("    GOFILES: %v\n", goFiles)
		fmt.Printf("    CGO:     %v\n", (len(cgoFiles) > 0))
	}

	cmd := (strings.Index(makefileText, "Make.cmd") != -1)
	pkg := (strings.Index(makefileText, "Make.pkg") != -1)
	var kind int
	switch {
	case cmd && !pkg:
		if *flag_debug {
			fmt.Printf("    kind:    cmd\n")
		}
		kind = MAKEFILE_CMD

	case !cmd && pkg:
		if *flag_debug {
			fmt.Printf("    kind:    pkg\n")
		}
		kind = MAKEFILE_PKG

	case cmd && pkg:
		return nil, os.NewError("the makefile \"" + path + "\" contains both 'Make.cmd' and 'Make.pkg'")

	default:
		if *flag_debug {
			fmt.Printf("    kind: unknown\n")
		}
		kind = MAKEFILE_UNKNOWN_TYPE
	}

	contents := &makefile_contents_t{
		targ:    targ,
		kind:    kind,
		goFiles: goFiles,
		cgo:     (len(cgoFiles) > 0),
	}

	return contents, nil
}
