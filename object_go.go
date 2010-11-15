package main

import (
	"bytes"
	"container/vector"
	"fmt"
	"go/ast"
	"go/parser"
	"io"
	"io/ioutil"
	"os"
	pathutil "path"
	"sort"
	"strings"
)

type go_source_code_t interface {
	object_t
	Parent() *dir_t
	Contents() (*go_file_contents_t, os.Error)
}

// Represents a FILE.go
type go_file_t struct {
	entry_t
	parent   *dir_t
	contents *go_file_contents_t // Initially nil
}

// Represents a FILE_test.go
type go_test_t struct {
	entry_t
	parent   *dir_t
	contents *go_file_contents_t // Initially nil
}

// Represents the main file for creating a test-executable
type go_testMain_t struct {
	entry_t
	parent     *dir_t
	contents   *go_file_contents_t // Initially nil
	importPath string
	refresh    bool
	tests      vector.StringVector
	benchmarks vector.StringVector
}

// The content of a Go file
type go_file_contents_t struct {
	packageName      string
	importedPackages []string
	tests            []string
	benchmarks       []string
}


// =========
// go_file_t
// =========

func new_go_file(entry entry_t, parent *dir_t) *go_file_t {
	f := &go_file_t{
		entry_t: entry,
		parent:  parent,
	}
	newObjects[f] = 0
	return f
}

func (f *go_file_t) Parent() *dir_t {
	return f.parent
}

func (f *go_file_t) Contents() (*go_file_contents_t, os.Error) {
	if f.contents == nil {
		var err os.Error
		f.contents, err = parse_go_file_contents(f.path, /*test*/ false)
		if err != nil {
			return nil, err
		}
	}

	return f.contents, nil
}

func (f *go_file_t) UpdateFileSystemModel() {
	f.UpdateFileInfo()
}

func inferObjects(f go_source_code_t, test bool) os.Error {
	if strings.HasSuffix(f.Path(), "_test/main.go") {
		return nil
	}

	contents, err := f.Contents()
	if err != nil {
		return err
	}

	if test && (contents.packageName == "main") {
		return os.NewError("cannot perform tests if the package is \"main\"")
	}

	// If there is no Makefile:
	//   If not in test mode:
	//    - expect a file like "_obj/PACKAGE.8"
	//    - register 'f' with the object "_obj/PACKAGE.8"
	//    - expect a file like "_obj/DIR/PACKAGE.a", or an executable
	//      - register the object "_obj/PACKAGE.8" with the object "_obj/DIR/PACKAGE.a", or
	//      - register the object "_obj/PACKAGE.8" with the executable
	//   If in test mode:
	//    - expect a file like "_test/PACKAGE.8"
	//    - register 'f' with the object "_test/PACKAGE.8"
	//    - expect a file like "_test/DIR/PACKAGE.a"
	//    - register the object "_test/PACKAGE.8" with the object "_test/DIR/PACKAGE.a"
	//    - expect file "_test/main.go"
	parent := f.Parent()
	if parent.makefile_orNil == nil {
		var objDir *dir_t
		if !test {
			objDir = parent.getOrCreateSubDir("_obj")
		} else {
			objDir = parent.getOrCreateSubDir("_test")
		}

		// File "_obj/PACKAGE.8" or "_test/PACKAGE.8"
		var compilationUnit *compilation_unit_t
		{
			var compilationUnit_name string
			if contents.packageName != "main" {
				compilationUnit_name = contents.packageName + o_ext
			} else {
				if pathFromMapping, haveMapping := source2executable[f.Path()]; haveMapping {
					compilationUnit_name = pathutil.Base(pathFromMapping) + o_ext
				} else {
					compilationUnit_name = contents.packageName + o_ext
				}
			}

			compilationUnit, err = objDir.getOrCreate_compilationUnit(compilationUnit_name)
			if err != nil {
				return err
			}

			// Register 'f' with the 'compilationUnit'
			compilationUnit.addSourceCode(f)
		}

		if contents.packageName != "main" {
			// If not test mode:
			//  - expect "_obj/DIR/PACKAGE.a"
			// If test mode:
			//  - "_test/DIR/PACKAGE.a"
			//  - expect file "_test/main.go"

			var target string
			{
				config := f.Parent().config_orNil
				if config == nil {
					return os.NewError("directory \"" + f.Parent().path + "\" requires a " + configFileName + " file" +
						" with a specification of the target package or executable")
				}

				target = config.targetPackage_orEmpty
				if len(target) == 0 {
					return os.NewError("config file for directory \"" + f.Parent().path + "\" does not specify the target package")
				}
			}

			dirPath, baseName := pathutil.Split(target)
			lib_dir := objDir.getOrCreateSubDirs(strings.Split(dirPath, "/", -1))
			lib_name := baseName + ".a"

			var lib *library_t
			lib, err = lib_dir.getOrCreate_library(lib_name)
			if err != nil {
				return err
			}

			// Register 'compilationUnit' with the 'library'
			lib.addCompilationUnit(compilationUnit)

			// Add 'lib' to the package resolution table
			err = mapImportPath(target, lib, objDir, nil, test)
			if err != nil {
				return err
			}

			if test {
				lib.partOfATest = true

				// Expect file "_test/main.go"
				var testMain *go_testMain_t
				testMain, err = objDir.getOrCreate_goTestMain("main.go", target, f.Parent())
				if err != nil {
					return err
				}

				testMain.addTests(contents.tests)
				testMain.addBenchmarks(contents.benchmarks)
			}
		} else {
			// Expect an executable

			var exe_name string
			var exe_dir *dir_t

			if pathFromMapping, haveMapping := source2executable[f.Path()]; haveMapping {
				dir, file := pathutil.Split(pathFromMapping)
				exe_name = file
				exe_dir = parent.root().getOrCreateSubDirs(strings.Split(dir, "/", -1))
			} else {
				exe_name = defaultExeName
				exe_dir = parent
			}

			var exe *executable_t
			exe, err = exe_dir.getOrCreate_executable(exe_name)
			if err != nil {
				return err
			}

			// Register 'compilationUnit' with the 'exe'
			exe.addCompilationUnit(compilationUnit)
		}
	}

	return nil
}

func (f *go_file_t) InferObjects(updateTests bool) os.Error {
	err := inferObjects(f, /*test*/ false)
	if err != nil {
		return err
	}

	if f.parent.numTestFiles > 0 {
		err = inferObjects(f, /*test*/ true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *go_file_t) PrintDependencies(w io.Writer) {
	return
}

func (f *go_file_t) Info(info *info_t) {
	return
}

func (f *go_file_t) Make() os.Error {
	return nil
}

func (f *go_file_t) MakeTests() os.Error {
	return nil
}

func (f *go_file_t) RunTests(testPattern, benchPattern string) os.Error {
	return nil
}

func (f *go_file_t) Clean() os.Error {
	remove := false
	if strings.HasPrefix(f.name, "_cgo_") || strings.HasSuffix(f.name, ".cgo1.go") {
		remove = true
	} else if f.name == "_testmain.go" {
		remove = true
	}

	if remove {
		var err os.Error
		if f.exists {
			if *flag_debug {
				println("remove:", f.path)
			}
			err = os.Remove(f.path)
			if err == nil {
				f.exists = false
			}
		} else {
			err = nil
		}
	}

	return nil
}

func (f *go_file_t) GoFmt() os.Error {
	if _, disabled := disabledGoFmt[pathutil.Clean(f.path)]; disabled {
		if *flag_debug {
			println("disabled gofmt:", f.path)
		}
		return nil
	}

	if strings.HasPrefix(f.name, "_cgo_") || strings.HasSuffix(f.name, ".cgo1.go") {
		if *flag_debug {
			println("no gofmt (cgo):", f.path)
		}
		return nil
	}

	return goFmt(f.path)
}


// =========
// go_test_t
// =========

func new_go_test(entry entry_t, parent *dir_t) *go_test_t {
	parent.numTestFiles += 1
	t := &go_test_t{
		entry_t: entry,
		parent:  parent,
	}
	newObjects[t] = 0
	return t
}

func (t *go_test_t) Parent() *dir_t {
	return t.parent
}

func (t *go_test_t) Contents() (*go_file_contents_t, os.Error) {
	if t.contents == nil {
		var err os.Error
		t.contents, err = parse_go_file_contents(t.path, /*test*/ true)
		if err != nil {
			return nil, err
		}
	}

	return t.contents, nil
}

func (t *go_test_t) UpdateFileSystemModel() {
	t.UpdateFileInfo()
}

func (t *go_test_t) InferObjects(updateTests bool) os.Error {
	return inferObjects(t, /*test*/ true)
}

func (t *go_test_t) PrintDependencies(w io.Writer) {
	return
}

func (t *go_test_t) Info(info *info_t) {
	return
}

func (t *go_test_t) Make() os.Error {
	return nil
}

func (t *go_test_t) MakeTests() os.Error {
	return nil
}

func (t *go_test_t) RunTests(testPattern, benchPattern string) os.Error {
	return nil
}

func (t *go_test_t) Clean() os.Error {
	return nil
}

func (t *go_test_t) GoFmt() os.Error {
	if _, disabled := disabledGoFmt[pathutil.Clean(t.path)]; disabled {
		if *flag_debug {
			println("disabled gofmt:", t.path)
		}
		return nil
	}

	return goFmt(t.path)
}


// =============
// go_testMain_t
// =============

func new_go_testMain(entry entry_t, parent *dir_t, importPath string) *go_testMain_t {
	t := &go_testMain_t{
		entry_t:    entry,
		parent:     parent,
		importPath: importPath,
		refresh:    true,
	}
	newObjects[t] = 0
	return t
}

func (t *go_testMain_t) setImportPath(importPath string) os.Error {
	if t.importPath != importPath {
		return os.NewError("failed to generate \"" + t.path + "\": inconsistent import paths")
	}

	return nil
}

func (t *go_testMain_t) addTests(names []string) {
	var vect vector.StringVector = names
	t.tests.AppendVector(&vect)
}

func (t *go_testMain_t) addBenchmarks(names []string) {
	var vect vector.StringVector = names
	t.benchmarks.AppendVector(&vect)
}

func (t *go_testMain_t) packageName() string {
	_, packageName := pathutil.Split(t.importPath)
	return packageName
}

func (t *go_testMain_t) refreshIfNeeded() os.Error {
	if t.refresh {
		// Get the current source code
		var oldContents []byte
		if t.exists {
			var err os.Error
			oldContents, err = ioutil.ReadFile(t.path)
			if err != nil {
				return err
			}
		} else {
			oldContents = nil
		}

		// Create the new source code
		var buf *bytes.Buffer
		{
			packageName := t.packageName()

			// Head
			buf = bytes.NewBuffer(make([]byte, 0, 300))
			buf.WriteString("package main\n")
			buf.WriteString("\n")
			if (len(t.tests) > 0) || (len(t.benchmarks) > 0) {
				buf.WriteString("import \"" + t.importPath + "\"\n")
			}
			buf.WriteString("import \"testing\"\n")
			buf.WriteString("import _regexp \"regexp\"\n")
			buf.WriteString("\n")

			// Tests
			buf.WriteString("var tests = []testing.InternalTest{\n")
			sort.SortStrings(t.tests)
			for _, test := range t.tests {
				qualifiedName := packageName + "." + test
				buf.WriteString("\t{\"" + qualifiedName + "\", " + qualifiedName + "},\n")
			}
			buf.WriteString("}\n")

			// Benchmarks
			buf.WriteString("var benchmarks = []testing.InternalBenchmark{\n")
			sort.SortStrings(t.benchmarks)
			for _, benchmark := range t.benchmarks {
				qualifiedName := packageName + "." + benchmark
				buf.WriteString("\t{\"" + qualifiedName + "\", " + qualifiedName + "},\n")
			}
			buf.WriteString("}\n")

			// Main func
			buf.WriteString("\n")
			buf.WriteString("func main() {\n")
			buf.WriteString("\ttesting.Main(_regexp.MatchString, tests)\n")
			buf.WriteString("\ttesting.RunBenchmarks(_regexp.MatchString, benchmarks)\n")
			buf.WriteString("}\n")
		}

		// Update the file if the new source code differs from the current one
		if (oldContents == nil) || (bytes.Equal(oldContents, buf.Bytes()) == false) {
			if *flag_debug {
				println("refresh:", t.path)
			}
			t.parent.mkdir_ifDoesNotExist()
			err := ioutil.WriteFile(t.path, buf.Bytes(), 0666)
			if err != nil {
				return err
			}

			t.UpdateFileInfo()
		}

		t.refresh = false
	}

	return nil
}

func (t *go_testMain_t) Parent() *dir_t {
	return t.parent
}

func (t *go_testMain_t) Contents() (*go_file_contents_t, os.Error) {
	err := t.refreshIfNeeded()
	if err != nil {
		return nil, err
	}

	if t.contents == nil {
		t.contents, err = parse_go_file_contents(t.path, /*test*/ false)
		if err != nil {
			return nil, err
		}
	}

	return t.contents, nil
}

func (t *go_testMain_t) UpdateFileSystemModel() {
	t.UpdateFileInfo()
}

func (t *go_testMain_t) InferObjects(updateTests bool) os.Error {
	var err os.Error

	// Generate and parse the source code
	if updateTests {
		_, err = t.Contents()
		if err != nil {
			return err
		}
	}

	// Expect file "main.8"
	var compilationUnit *compilation_unit_t
	{
		compilationUnit, err = t.parent.getOrCreate_compilationUnit("main" + o_ext)
		if err != nil {
			return err
		}

		// Register 't' with the 'compilationUnit'
		compilationUnit.addSourceCode(t)
	}

	// Expect executable "../package-test"
	var exe *executable_t
	{
		var exe_name string = testExeName
		var exe_dir *dir_t = t.parent.parent_orNil

		if exe_dir == nil {
			panic("the directory \"" + t.parent.path + "\" wasn't expected to be the root")
		}

		exe, err = exe_dir.getOrCreate_executable(exe_name)
		if err != nil {
			return err
		}

		// Register 'compilationUnit' with the 'exe'
		exe.addCompilationUnit(compilationUnit)
	}

	compilationUnit.testImportPath_orEmpty = t.importPath
	exe.testImportPath_orEmpty = t.importPath

	return nil
}

func (t *go_testMain_t) PrintDependencies(w io.Writer) {
	return
}

func (t *go_testMain_t) Info(info *info_t) {
	return
}

func (t *go_testMain_t) Make() os.Error {
	return nil
}

func (t *go_testMain_t) MakeTests() os.Error {
	return nil
}

func (t *go_testMain_t) RunTests(testPattern, benchPattern string) os.Error {
	return nil
}

func (t *go_testMain_t) Clean() os.Error {
	var err os.Error
	if t.exists {
		if *flag_debug {
			println("remove:", t.path)
		}
		err = os.Remove(t.path)
		if err == nil {
			t.exists = false
		}
	} else {
		err = nil
	}

	return err

}

func (t *go_testMain_t) GoFmt() os.Error {
	return nil
}


// ==================
// go_file_contents_t
// ==================

type ast_visitor_t struct {
	// A vector of 'ast.ImportSpec'
	importSpecs vector.Vector

	tests      vector.StringVector
	benchmarks vector.StringVector
}

func (v *ast_visitor_t) Visit(node interface{}) ast.Visitor {
	if importSpec, isImportSpec := node.(*ast.ImportSpec); isImportSpec {
		v.importSpecs.Push(importSpec)
		return nil
	} else if funcDecl, isFuncDecl := node.(*ast.FuncDecl); isFuncDecl {
		if funcDecl.Body != nil {
			name := funcDecl.Name.Name
			if strings.HasPrefix(name, "Test") {
				v.tests.Push(name)
			} else if strings.HasPrefix(name, "Benchmark") {
				v.benchmarks.Push(name)
			}
		}
		return nil
	}

	return v
}

func parse_go_file_contents(filePath string, test bool) (*go_file_contents_t, os.Error) {
	if *flag_debug {
		println("parse:", filePath)
	}

	var mode uint = 0
	if !test {
		mode = parser.ImportsOnly
	}

	var file *ast.File
	file, err := parser.ParseFile(filePath, /*src*/ nil, mode)
	if err != nil {
		return nil, err
	}

	v := ast_visitor_t{}
	ast.Walk(&v, file)

	// Extract imported packages
	var importedPackages []string
	{
		importedPackages = make([]string, v.importSpecs.Len())

		for i, importSpec := range v.importSpecs {
			// The value has format: DOUBLE-QUOTE .* DOUBLE-QUOTE
			val := string(importSpec.(*ast.ImportSpec).Path.Value)
			if (len(val) <= 2) || (val[0] != '"') || (val[len(val)-1] != '"') {
				// This should never happen
				return nil, os.NewError(filePath + ": an import spec lacks double-quotes")
			}

			// Strip the double-quotes
			val = val[1 : len(val)-1]

			importedPackages[i] = val
		}
	}

	contents := &go_file_contents_t{
		packageName:      file.Name.Name,
		importedPackages: importedPackages,
		tests:            v.tests,
		benchmarks:       v.benchmarks,
	}

	if *flag_debug {
		if !test {
			fmt.Printf("    package: %s\n", contents.packageName)
			fmt.Printf("    imports: %v\n", contents.importedPackages)
		} else {
			fmt.Printf("    package:    %s\n", contents.packageName)
			fmt.Printf("    imports:    %v\n", contents.importedPackages)
			fmt.Printf("    tests:      %s\n", contents.tests)
			fmt.Printf("    benchmarks: %s\n", contents.benchmarks)
		}
	}

	return contents, nil
}

func (f *go_file_contents_t) makePrerequisites(testImportPath_orEmpty string) ([]*package_resolution_t, os.Error) {
	pkgs := make([]*package_resolution_t, 0, len(f.importedPackages))

	for _, importedPackage := range f.importedPackages {
		test := (importedPackage == testImportPath_orEmpty)

		pkg, err := resolvePackage(importedPackage, test)
		if err != nil {
			return nil, err
		}

		if pkg != nil {
			err = pkg.lib.Make()
			if err != nil {
				return nil, err
			}

			// Append 'pkg' to 'pkgs'
			n := len(pkgs)
			pkgs = pkgs[0 : n+1]
			pkgs[n] = pkg
		}
	}

	return pkgs, nil
}
