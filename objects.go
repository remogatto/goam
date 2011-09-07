package main

import (
	"container/vector"
	"fmt"
	"io"
	"os"
	pathutil "path"
)

// A set of objects.
// This is a set, the map's value has no meaning.
var newObjects = make(map[object_t]byte)

type object_t interface {
	Name() string
	NameWithoutExtension() string
	Path() string
	Exists() bool
	Mtime() int64
	UpdateFileSystemModel()

	InferObjects(updateTests bool) os.Error
	PrintDependencies(w io.Writer)

	Info(info *info_t)
	Make() os.Error
	MakeTests() os.Error
	RunTests(testPattern, benchPattern string, errors *[]os.Error)
	Clean() os.Error
	GoFmt(files *vector.StringVector) os.Error
}

// A file system entry.
// This type is embedded in types defined below.
type entry_t struct {
	// Name of a file or directory.
	// This is the last path element of 'path'.
	name string

	// Path to a file or directory
	path string

	// Whether the file or directory specified by 'path' exists
	exists bool

	// File modification time (nanoseconds since epoch).
	// Only valid if 'exists' is true.
	mtime int64
}

// Represents a GOAM config file (GOAM.conf)
type config_file_t struct {
	entry_t
	parent                *dir_t
	targetPackage_orEmpty string // Empty string means the target package is unspecified
	packageFiles_orNil    map[string]byte // A set of file names, each name ends with ".go".
	                                      // A nil value means "all Go files in the directory".
}

// Represents a FILE.o, FILE.8, FILE.6, etc
type compilation_unit_t struct {
	entry_t
	parent                 *dir_t
	sources                []go_source_code_t
	testImportPath_orEmpty string
	built                  bool
	nowBuilding            bool
}

// Represents a static library (FILE.a)
type library_t struct {
	entry_t
	parent         *dir_t
	sources        []*compilation_unit_t
	makefile_orNil *makefile_t
	partOfATest    bool
	built          bool
	nowBuilding    bool
}

// Represents an executable
type executable_t struct {
	entry_t
	parent                 *dir_t
	sources                []*compilation_unit_t
	makefile_orNil         *makefile_t
	testImportPath_orEmpty string
	nowBuilding            bool
}


// =======
// entry_t
// =======

func new_entry(path string, fileInfo *os.FileInfo) entry_t {
	return entry_t{
		name:   fileInfo.Name,
		path:   path,
		exists: true,
		mtime:  fileInfo.Mtime_ns,
	}
}

func new_entry_from_path(name, path string) entry_t {
	fileInfo, err := os.Stat(path)
	if err == nil {
		return new_entry(path, fileInfo)
	}
	return new_nonexistent_entry(name, path)
}

func new_nonexistent_entry(name, path string) entry_t {
	return entry_t{
		name:   name,
		path:   path,
		exists: false,
		mtime:  -1,
	}
}

func (e *entry_t) Name() string {
	return e.name
}

func (e *entry_t) NameWithoutExtension() string {
	ext := pathutil.Ext(e.name)
	return e.name[0 : len(e.name)-len(ext)]
}

func (e *entry_t) Path() string {
	return e.path
}

func (e *entry_t) Exists() bool {
	return e.exists
}

func (e *entry_t) Mtime() int64 {
	if !e.exists {
		panic("the file \"" + e.path + "\" does not exist")
	}
	return e.mtime
}

func (e *entry_t) UpdateFileInfo() {
	fileInfo, err := os.Stat(e.path)
	if err == nil {
		e.exists = true
		e.mtime = fileInfo.Mtime_ns
	} else {
		e.exists = false
		e.mtime = -1
	}
}


// =============
// config_file_t
// =============

func new_config_file(entry entry_t, parent *dir_t) (*config_file_t, os.Error) {
	config := &config_file_t{
		entry_t: entry,
		parent:  parent,
	}

	if parent.config_orNil == nil {
		parent.config_orNil = config
	} else {
		return nil, os.NewError("directory \"" + parent.path + "\" contains multiple config files")
	}

	newObjects[config] = 0
	return config, nil
}

func (f *config_file_t) UpdateFileSystemModel() {
	f.UpdateFileInfo()
}

func (f *config_file_t) InferObjects(updateTests bool) os.Error {
	// Consistency check
	if f.packageFiles_orNil != nil {
		if len(f.targetPackage_orEmpty) == 0 {
			return os.NewError("configuration file \"" + f.path + "\" specifies package files, but does not specify the package")
		}
	}

	return nil
}

func (f *config_file_t) PrintDependencies(w io.Writer) {
	return
}

func (f *config_file_t) Info(info *info_t) {
	return
}

func (f *config_file_t) Make() os.Error {
	return nil
}

func (f *config_file_t) MakeTests() os.Error {
	return nil
}

func (f *config_file_t) RunTests(testPattern, benchPattern string, errors *[]os.Error) {
	return
}

func (f *config_file_t) Clean() os.Error {
	return nil
}

func (f *config_file_t) GoFmt(files *vector.StringVector) os.Error {
	return nil
}

func (f *config_file_t) ignoresGoFile(g go_source_code_t) bool {
	if f.packageFiles_orNil == nil {
		// Do not ignore any Go files
		return false
	}

	packageFiles := f.packageFiles_orNil

	_, present := packageFiles[g.Name()]
	return !present
}


// ==================
// compilation_unit_t
// ==================

func new_compilation_unit(entry entry_t, parent *dir_t) *compilation_unit_t {
	u := &compilation_unit_t{
		entry_t: entry,
		parent:  parent,
		sources: make([]go_source_code_t, 0, 8),
	}
	newObjects[u] = 0
	return u
}

func (u *compilation_unit_t) addSourceCode(src go_source_code_t) {
	for _, x := range u.sources {
		if x == src {
			// 'src' is already in 'list'
			return
		}
	}

	u.sources = append(u.sources, src)
}

func (u *compilation_unit_t) UpdateFileSystemModel() {
	u.UpdateFileInfo()
}

func (u *compilation_unit_t) InferObjects(updateTests bool) os.Error {
	return nil
}

func (u *compilation_unit_t) PrintDependencies(w io.Writer) {
	sources_paths := make([]string, len(u.sources))
	for i, src := range u.sources {
		sources_paths[i] = src.Path()
	}

	fmt.Fprintf(w, "%s <-- %v\n", u.path, sources_paths)
}

func (u *compilation_unit_t) Info(info *info_t) {
	return
}

func (u *compilation_unit_t) Make() os.Error {
	if u.built {
		return nil
	}

	if u.nowBuilding {
		return os.NewError("circular dependency involving \"" + u.path + "\"")
	}
	u.nowBuilding = true
	defer func() { u.nowBuilding = false }()

	rebuild := false
	if !u.exists {
		rebuild = true
	}

	var libIncludePaths map[*dir_t]byte = nil // This is a set of dirs

	{
		var missingSources []go_source_code_t = nil

		mtime := u.mtime
		for _, src := range u.sources {
			err := src.Make()
			if err != nil {
				return err
			}

			if !src.Exists() {
				if missingSources == nil {
					missingSources = make([]go_source_code_t, 0, len(u.sources))
				}
				missingSources = missingSources[0 : len(missingSources)+1]
				missingSources[len(missingSources)-1] = src
			} else if src.Mtime() > mtime {
				rebuild = true
			}

			var pkgs []*package_resolution_t
			{
				contents, err := src.Contents()
				if err != nil {
					return err
				}

				pkgs, err = contents.makePrerequisites(u.testImportPath_orEmpty)
				if err != nil {
					return err
				}
			}

			if len(pkgs) > 0 {
				if libIncludePaths == nil {
					libIncludePaths = make(map[*dir_t]byte)
				}
				for _, pkg := range pkgs {
					libIncludePaths[pkg.includePath] = 0

					if pkg.lib.Mtime() > mtime {
						rebuild = true
					}
				}
			}
		}

		if len(missingSources) != 0 {
			missing := make([]string, len(missingSources))
			for i, src := range missingSources {
				missing[i] = src.Path()
			}
			msg := fmt.Sprintf("unable to build \"%s\": missing files %v", u.path, missing)
			return os.NewError(msg)
		}
	}

	if rebuild {
		err := u.parent.mkdir_ifDoesNotExist()
		if err != nil {
			return err
		}

		var args []string
		{
			args = append(args, goCompiler_exe.name)
			args = append(args, goCompiler_flags...)
			args = append(args, "-o")
			args = append(args, u.path)
			if libIncludePaths != nil {
				for incPath, _ := range libIncludePaths {
					args = append(args, "-I", incPath.path)
				}
			}
			for _, src := range u.sources {
				args = append(args, src.Path())
			}
		}

		err = goCompiler_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
		if err != nil {
			return err
		}

		u.UpdateFileInfo()
		if !u.exists {
			return os.NewError("failed to build \"" + u.path + "\"")
		}
	}

	u.built = true
	return nil
}

func (u *compilation_unit_t) MakeTests() os.Error {
	return nil
}

func (u *compilation_unit_t) RunTests(testPattern, benchPattern string, errors *[]os.Error) {
	return
}

func (u *compilation_unit_t) Clean() os.Error {
	var err os.Error
	if u.exists {
		if *flag_debug {
			println("remove:", u.path)
		}
		err = os.Remove(u.path)
		if err == nil {
			u.exists = false
		}
	} else {
		err = nil
	}

	return err
}

func (u *compilation_unit_t) GoFmt(files *vector.StringVector) os.Error {
	return nil
}


// =========
// library_t
// =========

func new_library(entry entry_t, parent *dir_t) *library_t {
	l := &library_t{
		entry_t: entry,
		parent:  parent,
		sources: make([]*compilation_unit_t, 0, 2),
	}
	newObjects[l] = 0
	return l
}

func (l *library_t) addCompilationUnit(u *compilation_unit_t) {
	for _, x := range l.sources {
		if x == u {
			// 'u' is already in 'l.sources'
			return
		}
	}

	l.sources = append(l.sources, u)
}

func (l *library_t) addMakefile(m *makefile_t) os.Error {
	if l.makefile_orNil != nil {
		return os.NewError("library \"" + l.path + "\" is a product of more than one Makefile")
	}

	l.makefile_orNil = m
	return nil
}

func (l *library_t) UpdateFileSystemModel() {
	l.UpdateFileInfo()
}

func (l *library_t) InferObjects(updateTests bool) os.Error {
	return nil
}

func (l *library_t) PrintDependencies(w io.Writer) {
	sources_paths := make([]string, len(l.sources))
	for i, src := range l.sources {
		sources_paths[i] = src.Path()
	}
	fmt.Fprintf(w, "%s <-- %v", l.path, sources_paths)

	if l.makefile_orNil != nil {
		fmt.Fprintf(w, ", makefile \"%s\"", l.makefile_orNil.path)
	}

	fmt.Fprintf(w, "\n")
}

func (l *library_t) Info(info *info_t) {
	if l.partOfATest {
		return
	}

	if (len(l.sources) > 0) || (l.makefile_orNil != nil) {
		info.libs[l] = 0
	}
}

func (l *library_t) Make() os.Error {
	if l.built {
		return nil
	}

	if l.nowBuilding {
		return os.NewError("circular dependency involving \"" + l.path + "\"")
	}
	l.nowBuilding = true
	defer func() { l.nowBuilding = false }()

	rebuild := false
	if !l.exists {
		rebuild = true
	}

	{
		mtime := l.mtime
		for _, src := range l.sources {
			err := src.Make()
			if err != nil {
				return err
			}

			if src.Mtime() > mtime {
				rebuild = true
			}
		}
	}

	if rebuild {
		if l.makefile_orNil == nil {
			err := l.parent.mkdir_ifDoesNotExist()
			if err != nil {
				return err
			}

			if l.exists {
				if *flag_debug {
					println("remove:", l.path)
				}
				err := os.Remove(l.path)
				if err != nil {
					return err
				}
			}

			var args []string
			args = append(args, goArchiver_exe.name)
			args = append(args, goArchiver_flags...)
			args = append(args, l.path)
			for _, src := range l.sources {
				args = append(args, src.Path())
			}

			err = goArchiver_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
			if err != nil {
				return err
			}

			l.UpdateFileInfo()
			if !l.exists {
				return os.NewError("failed to build \"" + l.path + "\"")
			}
		} else {
			err := l.makefile_orNil.Make()
			if err != nil {
				return err
			}

			if !l.exists {
				return os.NewError("failed to build \"" + l.path + "\"")
			}
		}
	}

	l.built = true
	return nil
}

func (l *library_t) MakeTests() os.Error {
	return nil
}

func (l *library_t) RunTests(testPattern, benchPattern string, errors *[]os.Error) {
	return
}

func (l *library_t) Clean() os.Error {
	var err os.Error
	if l.exists {
		if *flag_debug {
			println("remove:", l.path)
		}
		err = os.Remove(l.path)
		if err == nil {
			l.exists = false
		}
	} else {
		err = nil
	}

	return err
}

func (l *library_t) Install(importPath string) os.Error {
	err := l.Make()
	if err != nil {
		return err
	}

	dir, base := pathutil.Split(importPath)

	dstFullPath := pathutil.Join(libInstallRoot, dir)

	err = mkdirAll(dstFullPath, 0777)
	if err != nil {
		return err
	}

	installPath := pathutil.Join(dstFullPath, base+".a")

	args := []string{cp_exe.name, "-a", l.path, installPath}
	err = cp_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
	if err != nil {
		return err
	}

	return nil
}

func (l *library_t) Uninstall(importPath string) os.Error {
	dir, base := pathutil.Split(importPath)

	installPath := pathutil.Join(libInstallRoot, dir, base+".a")

	if fileExists(installPath) {
		if *flag_debug {
			println("uninstall:", installPath)
		}

		err := os.Remove(installPath)
		if err != nil {
			return err
		}
	}

	err := uninstallEmptyDirs(libInstallRoot, dir)
	if err != nil {
		return err
	}

	return nil
}

func (l *library_t) GoFmt(files *vector.StringVector) os.Error {
	return nil
}


// ============
// executable_t
// ============

func new_executable(entry entry_t, parent *dir_t) *executable_t {
	e := &executable_t{
		entry_t: entry,
		parent:  parent,
		sources: make([]*compilation_unit_t, 0, 2),
	}
	newObjects[e] = 0
	return e
}

func (e *executable_t) addCompilationUnit(u *compilation_unit_t) {
	for _, x := range e.sources {
		if x == u {
			// 'u' is already in 'e.sources'
			return
		}
	}

	e.sources = append(e.sources, u)
}

func (e *executable_t) addMakefile(m *makefile_t) os.Error {
	if e.makefile_orNil != nil {
		return os.NewError("executable \"" + e.path + "\" is a product of more than one Makefile")
	}

	e.makefile_orNil = m
	return nil
}

func (e *executable_t) collectLibs() ([]*dir_t, os.Error) {
	var imports = make(map[string]*package_resolution_t)

	// The set of import statements to process
	var todo = make(map[string]byte)

	// Initialize 'todo' from 'e.sources'
	var compilationUnit *compilation_unit_t
	for _, compilationUnit = range e.sources {
		var goSourceCode go_source_code_t
		for _, goSourceCode = range compilationUnit.sources {
			contents, err := goSourceCode.Contents()
			if err != nil {
				return nil, err
			}

			for _, importedPackage := range contents.importedPackages {
				todo[importedPackage] = /*arbitrary value*/ 0
			}
		}
	}

	// Iterate until 'todo' is empty
	for len(todo) > 0 {
		var todo2 = make(map[string]byte)

		for importPath, _ := range todo {
			test := (importPath == e.testImportPath_orEmpty)
			pkg_orNil, err := resolvePackage(importPath, test)
			if err != nil {
				return nil, err
			}

			imports[importPath] = pkg_orNil

			if pkg_orNil != nil {
				var lib *library_t = pkg_orNil.lib

				// Add 'lib.sources' to 'todo2'
				var compilationUnit *compilation_unit_t
				for _, compilationUnit = range lib.sources {
					var goSourceCode go_source_code_t
					for _, goSourceCode = range compilationUnit.sources {
						contents, err := goSourceCode.Contents()
						if err != nil {
							return nil, err
						}

						for _, importedPackage := range contents.importedPackages {
							if _, alreadyProcessed := imports[importedPackage]; alreadyProcessed {
								continue
							}
							if _, alreadyProcessed := todo[importedPackage]; alreadyProcessed {
								continue
							}

							todo2[importedPackage] = /*arbitrary value*/ 0
						}
					}
				}
			}
		}

		todo = todo2
	}

	var libIncludePaths = make([]*dir_t, len(imports))
	{
		i := 0
		for _, pkg_orNil := range imports {
			if pkg_orNil != nil {
				pkg := pkg_orNil

				libIncludePaths[i] = pkg.includePath
				i++
			}
		}

		libIncludePaths = libIncludePaths[0:i]
	}

	return libIncludePaths, nil
}

func (e *executable_t) UpdateFileSystemModel() {
	e.UpdateFileInfo()
}

func (e *executable_t) InferObjects(updateTests bool) os.Error {
	return nil
}

func (e *executable_t) PrintDependencies(w io.Writer) {
	sources_paths := make([]string, len(e.sources))
	for i, src := range e.sources {
		sources_paths[i] = src.Path()
	}
	fmt.Fprintf(w, "%s <-- %v", e.path, sources_paths)

	if e.makefile_orNil != nil {
		fmt.Fprintf(w, ", makefile \"%s\"", e.makefile_orNil.path)
	}

	fmt.Fprintf(w, "\n")
}

func (e *executable_t) Info(info *info_t) {
	if len(e.testImportPath_orEmpty) != 0 {
		info.tests[e] = 0
	} else if (len(e.sources) > 0) || (e.makefile_orNil != nil) {
		info.executables[e] = 0
	}
}

func (e *executable_t) doMake(installMode bool) os.Error {
	var err os.Error

	if e.nowBuilding {
		return os.NewError("circular dependency involving \"" + e.path + "\"")
	}
	e.nowBuilding = true
	defer func() { e.nowBuilding = false }()

	var libIncludePaths []*dir_t
	libIncludePaths, err = e.collectLibs()
	if err != nil {
		return err
	}

	rebuild := false
	if !e.exists {
		rebuild = true
	}

	// Build all prerequisites
	{
		mtime := e.mtime

		for _, src := range e.sources {
			err = src.Make()
			if err != nil {
				return err
			}

			if src.Mtime() > mtime {
				rebuild = true
			}
		}
	}

	if rebuild || installMode {
		if e.makefile_orNil == nil {
			err = e.parent.mkdir_ifDoesNotExist()
			if err != nil {
				return err
			}

			var args []string
			{
				var target string
				if !installMode {
					target = e.path
				} else {
					target = pathutil.Join(exeInstallDir, e.name)
				}

				args = append(args, goLinker_exe.name)
				args = append(args, "-o")
				args = append(args, target)
				for _, incPath := range libIncludePaths {
					args = append(args, "-L", incPath.path)
				}
				for _, src := range e.sources {
					args = append(args, src.Path())
				}
			}

			err = goLinker_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
			if err != nil {
				return err
			}

			if !installMode {
				e.UpdateFileInfo()
				if !e.exists {
					return os.NewError("failed to build \"" + e.path + "\"")
				}
			}
		} else {
			err = e.makefile_orNil.Make()
			if err != nil {
				return err
			}

			if installMode {
				err = e.makefile_orNil.MakeInstall()
				if err != nil {
					return err
				}
			}

			if !e.exists {
				return os.NewError("failed to build \"" + e.path + "\"")
			}
		}
	}

	return nil
}

func (e *executable_t) Make() os.Error {
	// If 'e' is not a test/benchmark
	if len(e.testImportPath_orEmpty) == 0 {
		err := e.doMake( /*installMode*/ false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *executable_t) MakeTests() os.Error {
	// If 'e' is a test/benchmark
	if len(e.testImportPath_orEmpty) > 0 {
		err := e.doMake( /*installMode*/ false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *executable_t) RunTests(testPattern, benchPattern string, errors *[]os.Error) {
	// If 'e' is a test
	if len(e.testImportPath_orEmpty) > 0 {
		exe := &Executable{name: "./" + e.name, noLookup: true}

		args := make([]string, 1, 4)
		args[0] = exe.name
		if len(testPattern) > 0 {
			args = append(args, "-test.run="+testPattern)
		}
		if len(benchPattern) > 0 {
			args = append(args, "-test.bench="+benchPattern)
		}
		if *flag_verbose {
			args = append(args, "-test.v")
		}

		err := exe.runSimply(args, /*dir*/ e.parent.path, /*dontPrint*/ false)
		if err != nil {
			*errors = append(*errors, err)
		}
	}
}

func (e *executable_t) Clean() os.Error {
	var err os.Error
	if e.exists && (len(e.sources) >= 1) {
		if *flag_debug {
			println("remove:", e.path)
		}
		err = os.Remove(e.path)
		if err == nil {
			e.exists = false
		}
	} else {
		err = nil
	}

	return err
}

func (e *executable_t) Install() os.Error {
	// Error if 'e' is a test/benchmark
	if len(e.testImportPath_orEmpty) != 0 {
		return os.NewError("cannot install executable \"" + e.path + "\" because it is a test")
	}

	err := e.doMake( /*installMode*/ true)
	if err != nil {
		return err
	}

	return nil

}

func (e *executable_t) Uninstall() os.Error {
	installPath := pathutil.Join(exeInstallDir, e.name)

	// Delete the file (if it exists)
	if fileExists(installPath) {
		if *flag_debug {
			println("uninstall:", installPath)
		}

		err := os.Remove(installPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *executable_t) GoFmt(files *vector.StringVector) os.Error {
	return nil
}
