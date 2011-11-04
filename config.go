package main

import (
	"bytes"
	eval "bitbucket.org/binet/go-eval/pkg/eval"
	"errors"
	"fmt"
	"go/token"
	"io/ioutil"
	pathutil "path"
	"strings"
	"sync"
)

const (
	configFileName = "GOAM.conf"
	defaultExeName = "a.out"
	testExeName    = "package-test"
)

var currentConfig *config_file_t = nil
var configCurrent_mutex sync.Mutex

// Reads the specified config file
func readConfig(config *config_file_t) error {
	var err error

	configCurrent_mutex.Lock()
	{
		if *flag_debug {
			println("read config:", config.path)
		}

		currentConfig = config

		w := eval.NewWorld()
		defineConstants(w)
		defineFunctions(w)
		err = loadAndRunScript(w, config.path)

		currentConfig = nil
	}
	configCurrent_mutex.Unlock()

	if err != nil {
		err = errors.New(config.Path() + ": " + err.Error())
	}

	return err
}

// Loads and evaluates the specified Go script
func loadAndRunScript(w *eval.World, path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Write(data)
	return runScript(w, path, buf.String())
}

// Runs the specified Go source code in the context of 'w'
func runScript(w *eval.World, path, sourceCode string) error {
	var err error
	var code eval.Code

	fileSet := token.NewFileSet()
	fileSet.AddFile(path, fileSet.Base(), len(sourceCode))

	code, err = w.Compile(fileSet, sourceCode)
	if err != nil {
		str := strings.Replace(err.Error(), "input", path, 1)
		return errors.New(str)
	}

	_, err = code.Run()
	if err != nil {
		return err
	}

	return nil
}

// An implementation of 'eval.StringValue'
type string_value_t string

func (v *string_value_t) String() string {
	return string(*v)
}

func (v *string_value_t) Assign(t *eval.Thread, o eval.Value) {
	*v = string_value_t(o.(eval.StringValue).Get(t))
}

func (v *string_value_t) Get(*eval.Thread) string {
	return string(*v)
}

func (v *string_value_t) Set(t *eval.Thread, x string) {
	*v = string_value_t(x)
}

func defineConstants(w *eval.World) {
	GOOS := string_value_t(*flag_os)
	GOARCH := string_value_t(*flag_arch)
	GO_COMPILER := string_value_t(goCompiler_name)

	w.DefineConst("GOOS", eval.StringType, &GOOS)
	w.DefineConst("GOARCH", eval.StringType, &GOARCH)
	w.DefineConst("GO_COMPILER", eval.StringType, &GO_COMPILER)
}

func defineFunctions(w *eval.World) {
	{
		var functionSignature func(string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_Package, functionSignature)
		w.DefineVar("Package", funcType, funcValue)
	}

	{
		var functionSignature func(string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_PackageFiles, functionSignature)
		w.DefineVar("PackageFiles", funcType, funcValue)
	}

	{
		var functionSignature func(string, string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_Executable, functionSignature)
		w.DefineVar("Executable", funcType, funcValue)
	}

	{
		var functionSignature func(string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_IgnoreDir, functionSignature)
		w.DefineVar("IgnoreDir", funcType, funcValue)
	}

	{
		var functionSignature func(string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_DisableGoFmt, functionSignature)
		w.DefineVar("DisableGoFmt", funcType, funcValue)
	}

	{
		var functionSignature func(uint)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_MinGoamVersion, functionSignature)
		w.DefineVar("MinGoamVersion", funcType, funcValue)
	}

	{
		var functionSignature func(uint)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_MinCompilerVersion, functionSignature)
		w.DefineVar("MinCompilerVersion", funcType, funcValue)
	}

	{
		var functionSignature func()
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_InstallPackage, functionSignature)
		w.DefineVar("InstallPackage", funcType, funcValue)
	}

	{
		var functionSignature func(string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_InstallExecutable, functionSignature)
		w.DefineVar("InstallExecutable", funcType, funcValue)
	}

	{
		var functionSignature func(string, string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_InstallDir, functionSignature)
		w.DefineVar("InstallDir", funcType, funcValue)
	}

	{
		var functionSignature func(string, string, string, []string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_RemotePackage, functionSignature)
		w.DefineVar("RemotePackage", funcType, funcValue)
	}
}

// Signature: func Package(path string)
func wrapper_Package(t *eval.Thread, in []eval.Value, out []eval.Value) {
	pkg := in[0].(eval.StringValue).Get(t)

	if len(currentConfig.targetPackage_orEmpty) != 0 {
		t.Abort(errors.New("duplicate target package specification"))
		return
	}

	pkg = strings.TrimSpace(pkg)
	if len(pkg) == 0 {
		t.Abort(errors.New("the target package cannot be an empty string"))
		return
	}

	if *flag_debug {
		println("(read config) target package = \"" + pkg + "\"")
	}
	currentConfig.targetPackage_orEmpty = pkg
}

// Signature: func PackageFiles(files string)
func wrapper_PackageFiles(t *eval.Thread, in []eval.Value, out []eval.Value) {
	_files := in[0].(eval.StringValue).Get(t)

	if currentConfig.packageFiles_orNil != nil {
		t.Abort(errors.New("package files already defined"))
		return
	}

	var files []string
	{
		files = strings.Fields(_files)
		if len(files) == 0 {
			t.Abort(errors.New("empty list of files"))
			return
		}

		// Check 'files[i]'
		for i := 0; i < len(files); i++ {
			file, err := cleanAndCheckPath(t, files[i])
			if err != nil {
				t.Abort(err)
				return
			}

			if !strings.HasSuffix(file, ".go") {
				t.Abort(errors.New("the name of file \"" + file + "\" does not end with \".go\""))
				return
			}

			dir, _ := pathutil.Split(file)
			if len(dir) > 0 {
				t.Abort(errors.New("package file \"" + file + "\" uses a relative path"))
				return
			}

			path := pathutil.Join(currentConfig.parent.path, file)

			if !fileExists(path) {
				t.Abort(errors.New("file \"" + path + "\" does not exist"))
				return
			}

			files[i] = file
		}
	}

	if *flag_debug {
		fmt.Printf("(read config) package files %v\n", files)
	}

	packageFiles := make(map[string]byte)
	for _, file := range files {
		packageFiles[file] = 0
	}
	currentConfig.packageFiles_orNil = packageFiles
}

// Mapping between [the path of an executable] and [the paths of Go files from which to build the executable]
var executable2sources = make(map[string][]string)
var source2executable = make(map[string]string)

// Signature: func Executable(name string, sources string)
func wrapper_Executable(t *eval.Thread, in []eval.Value, out []eval.Value) {
	name := in[0].(eval.StringValue).Get(t)
	_sources := in[1].(eval.StringValue).Get(t)

	// Check the name, make the name relative to the local root
	{
		var err error
		name, err = cleanAndCheckPath(t, name)
		if err != nil {
			t.Abort(err)
			return
		}

		_, file := pathutil.Split(name)
		if file == testExeName {
			t.Abort(errors.New("executables named \"" + name + "\" are used for tests"))
			return
		}

		name = pathutil.Join(currentConfig.parent.path, name)
		if _, alreadyPresent := executable2sources[name]; alreadyPresent {
			t.Abort(errors.New("duplicate executable \"" + name + "\""))
			return
		}
	}

	var sources []string
	{
		sources = strings.Fields(_sources)
		if len(sources) == 0 {
			t.Abort(errors.New("empty list of sources"))
			return
		}

		// Check 'sources[i]', make 'sources[i]' relative to the local root
		for i := 0; i < len(sources); i++ {
			source, err := cleanAndCheckPath(t, sources[i])
			if err != nil {
				t.Abort(err)
				return
			}

			if !strings.HasSuffix(source, ".go") {
				t.Abort(errors.New("the name of file \"" + source + "\" does not end with \".go\""))
				return
			}

			source = pathutil.Join(currentConfig.parent.path, source)

			if _, alreadyPresent := source2executable[source]; alreadyPresent {
				t.Abort(errors.New("cannot associate file \"" + source + "\" with more than one executable"))
				return
			}

			if !fileExists(source) {
				t.Abort(errors.New("executable \"" + name + "\" depends on non-existent file \"" + source + "\""))
				return
			}

			sources[i] = source
		}
	}

	if *flag_debug {
		fmt.Printf("(read config) exe \"%s\" <-- %v\n", name, sources)
	}

	executable2sources[name] = sources
	for _, source := range sources {
		source2executable[source] = name
	}
}

// Checks the existence and contents of Go source code files
// associated with all executables in 'executable2sources'
func check_executable2sources(root *dir_t) error {
	for executable, sources := range executable2sources {
		for _, source := range sources {
			var object object_t = root.getObject_orNil(strings.Split(source, "/"))

			if object == nil {
				return errors.New("executable \"" + executable + "\" depends on non-existent object \"" + source + "\"")
			}

			if src, ok := object.(go_source_code_t); ok {
				contents, err := src.Contents()
				if err != nil {
					return err
				}

				if contents.packageName != "main" {
					return errors.New("file \"" + source + "\" (associated with executable \"" + executable + "\") " +
						"does not belong to package \"main\"")
				}
			} else {
				return errors.New("executable \"" + executable + "\" depends on \"" + source + "\", " +
					"but \"" + source + "\" is not a Go source code file")
			}
		}
	}

	return nil
}

func cleanAndCheckPath(t *eval.Thread, path string) (string, error) {
	path = pathutil.Clean(path)

	if len(path) == 0 {
		return "", errors.New("empty path")
	}
	if pathutil.IsAbs(path) {
		return "", errors.New("path \"" + path + "\" is not a relative path")
	}
	if strings.HasPrefix(path, "..") {
		return "", errors.New("path \"" + path + "\" is referring the parental directory")
	}

	return path, nil
}

// Set of directories to ignore.
// This is a set, the values of this hash-map have no meaning.
var ignoredDirs = make(map[string]byte)

// Signature: func IgnoreDir(path string)
func wrapper_IgnoreDir(t *eval.Thread, in []eval.Value, out []eval.Value) {
	path := in[0].(eval.StringValue).Get(t)

	var err error
	path, err = cleanAndCheckPath(t, path)
	if err != nil {
		t.Abort(err)
		return
	}

	path = pathutil.Join(currentConfig.parent.path, path)

	if *flag_debug {
		println("(read config) ignore dir \"" + path + "\"")
	}
	ignoredDirs[path] = 0
}

// Set of files for which gofmt is disabled.
// This is a set, the values of this hash-map have no meaning.
var disabledGoFmt = make(map[string]byte)

// Signature: func DisableGoFmt(path string)
func wrapper_DisableGoFmt(t *eval.Thread, in []eval.Value, out []eval.Value) {
	path := in[0].(eval.StringValue).Get(t)

	var err error
	path, err = cleanAndCheckPath(t, path)
	if err != nil {
		t.Abort(err)
		return
	}

	path = pathutil.Join(currentConfig.parent.path, path)

	if _, alreadyPresent := disabledGoFmt[path]; alreadyPresent {
		t.Abort(errors.New("gofmt already disabled: \"" + path + "\""))
		return
	}

	if *flag_debug {
		println("(read config) disable gofmt \"" + path + "\"")
	}
	disabledGoFmt[path] = 0
}

const VERSION = 2

// Signature: func MinGoamVersion(version uint)
func wrapper_MinGoamVersion(t *eval.Thread, in []eval.Value, out []eval.Value) {
	minVersion := in[0].(eval.UintValue).Get(t)

	if *flag_debug {
		println("(read config) Goam min version:", minVersion)
	}

	if VERSION < minVersion {
		msg := fmt.Sprintf("insufficient GOAM version: %d, minimum required version is %d", VERSION, minVersion)
		t.Abort(errors.New(msg))
		return
	}
}

// Signature: func MinCompilerVersion(version uint)
func wrapper_MinCompilerVersion(t *eval.Thread, in []eval.Value, out []eval.Value) {
	minVersion := in[0].(eval.UintValue).Get(t)

	if *flag_debug {
		println("(read config) Go compiler min version:", minVersion)
	}

	if *flag_gcc {
		t.Abort(errors.New("function MinCompilerVersion is incompatible with gccgo"))
		return
	}

	version, err := getGoCompilerVersion()
	if err != nil {
		t.Abort(err)
		return
	}

	if uint64(version) < minVersion {
		msg := fmt.Sprintf("insufficient Go compiler version: %d, minimum required version is %d", version, minVersion)
		t.Abort(errors.New(msg))
		return
	}
}

// Signature: func InstallPackage()
func wrapper_InstallPackage(t *eval.Thread, in []eval.Value, out []eval.Value) {
	pkg := currentConfig.targetPackage_orEmpty
	if len(pkg) == 0 {
		t.Abort(errors.New("no target package has been defined"))
		return
	}

	if _, alreadyPresent := installationCommands_packagesByImport[pkg]; alreadyPresent {
		t.Abort(errors.New("duplicate installation of package \"" + pkg + "\""))
		return
	}

	if *flag_debug {
		fmt.Printf("(read config) install package \"" + pkg + "\"\n")
	}

	cmd := new_installPackage(pkg)
	installationCommands = append(installationCommands, cmd)
	installationCommands_packagesByImport[pkg] = cmd
}

// Signature: func InstallExecutable(srcPath string)
func wrapper_InstallExecutable(t *eval.Thread, in []eval.Value, out []eval.Value) {
	srcPath := in[0].(eval.StringValue).Get(t)

	var err error

	// Check the 'srcPath', make it relative to the local root
	{
		srcPath, err = cleanAndCheckPath(t, srcPath)
		if err != nil {
			t.Abort(err)
			return
		}

		_, file := pathutil.Split(srcPath)
		if file == testExeName {
			t.Abort(errors.New("cannot install: executables named \"" + srcPath + "\" are used for tests"))
			return
		}

		srcPath = pathutil.Join(currentConfig.parent.path, srcPath)
		if _, alreadyPresent := installationCommands_bySrcPath[srcPath]; alreadyPresent {
			t.Abort(errors.New("duplicate installation of \"" + srcPath + "\""))
			return
		}
	}

	if *flag_debug {
		fmt.Printf("(read config) install exe \"%s\"\n", srcPath)
	}

	cmd := new_installExecutable(srcPath)
	installationCommands = append(installationCommands, cmd)
	installationCommands_bySrcPath[srcPath] = cmd
}

// Signature: func InstallDir(srcPath, dstPath string)
func wrapper_InstallDir(t *eval.Thread, in []eval.Value, out []eval.Value) {
	srcPath := in[0].(eval.StringValue).Get(t)
	dstPath := in[1].(eval.StringValue).Get(t)

	var err error

	// Check the 'srcPath', make it relative to the local root
	{
		srcPath, err = cleanAndCheckPath(t, srcPath)
		if err != nil {
			t.Abort(err)
			return
		}

		srcPath = pathutil.Join(currentConfig.parent.path, srcPath)
		if _, alreadyPresent := installationCommands_bySrcPath[srcPath]; alreadyPresent {
			t.Abort(errors.New("duplicate installation of \"" + srcPath + "\""))
			return
		}
	}

	// Clean the 'dstPath'
	dstPath = pathutil.Clean(dstPath)

	if *flag_debug {
		fmt.Printf("(read config) install dir \"%s\" --> \"%s\"\n", srcPath, dstPath)
	}

	cmd := new_installDir(srcPath, dstPath)
	installationCommands = append(installationCommands, cmd)
	installationCommands_bySrcPath[srcPath] = cmd
}

// Signature: func RemotePackage(importPaths, type, repository string, installCommand []string)
func wrapper_RemotePackage(t *eval.Thread, in []eval.Value, out []eval.Value) {
	importPaths := in[0].(eval.StringValue).Get(t)
	kindString := in[1].(eval.StringValue).Get(t)
	repositoryPath := in[2].(eval.StringValue).Get(t)
	installCommand_sliceValue := in[3].(eval.SliceValue).Get(t)

	var importPaths_array []string
	{
		importPaths_array = strings.Split(importPaths, " ")

		// Remove empty strings from 'importPaths_array'
		{
			i, j := 0, 0
			for i < len(importPaths_array) {
				if len(importPaths_array[i]) != 0 {
					importPaths_array[j] = importPaths_array[i]
					i++
					j++
				} else {
					i++
				}
			}

			importPaths_array = importPaths_array[0:j]
		}

		if len(importPaths_array) == 0 {
			t.Abort(errors.New("repository \"" + repositoryPath + "\": empty list of import paths"))
			return
		}

		// Check for duplicated import paths
		if len(importPaths_array) > 1 {
			importPaths_set := make(map[string]byte)
			for _, importPath := range importPaths_array {
				if _, alreadyExists := importPaths_set[importPath]; alreadyExists {
					t.Abort(errors.New("repository \"" + repositoryPath + "\": duplicate import path \"" + importPath + "\""))
					return
				}

				importPaths_set[importPath] = 0
			}
		}
	}

	var kind int
	switch strings.ToLower(kindString) {
	case "github":
		kind = GITHUB
	case "bitbucket":
		kind = BITBUCKET
	default:
		t.Abort(errors.New("repository \"" + repositoryPath + "\": \"" + kindString + "\" is an invalid repository type"))
		return
	}

	// Check 'repositoryPath'
	{
		repositoryPath = strings.TrimSpace(repositoryPath)

		err := checkRepositoryPath(kind, repositoryPath)
		if err != nil {
			t.Abort(err)
			return
		}
	}

	var installCommand []string
	{
		var array eval.ArrayValue = installCommand_sliceValue.Base
		var length int64 = installCommand_sliceValue.Len

		installCommand = make([]string, length)
		for i := int64(0); i < length; i++ {
			installCommand[i] = array.Elem(t, i).(eval.StringValue).Get(t)
		}

		if len(installCommand) == 0 {
			t.Abort(errors.New("repository \"" + repositoryPath + "\": empty installation command"))
			return
		}
	}

	if *flag_debug {
		fmt.Printf("(read config) remote package: %v %s \"%s\"\n", importPaths_array, kindString, repositoryPath)
	}

	// Find or create an instance of 'remote_package_t'
	var remotePkg *remote_package_t
	{
		if remotePkg1, alreadyExists := remotePackages_byRepository[repositoryPath]; alreadyExists {
			if remotePkg1.repository.Kind() != kind {
				t.Abort(errors.New("repository \"" + repositoryPath + "\" redefined with a different repository type"))
				return
			}

			if len(remotePkg1.installCmd) != len(installCommand) {
				t.Abort(errors.New("repository \"" + repositoryPath + "\" redefined with a different installation command"))
				return
			}
			for i := 0; i < len(installCommand); i++ {
				if remotePkg1.installCmd[i] != installCommand[i] {
					t.Abort(errors.New("repository \"" + repositoryPath + "\" redefined with a different installation command"))
					return
				}
			}

			remotePkg = remotePkg1
		} else {
			// Create a new object conforming to the interface 'repository_t'
			var repository repository_t
			switch kind {
			case GITHUB:
				repository = new_repository_github(repositoryPath)
			case BITBUCKET:
				repository = new_repository_bitbucket(repositoryPath)
			default:
				panic("invalid kind")
			}

			// Create a new 'remote_package_t'
			remotePkg = new_remotePackage(importPaths_array, repository, installCommand)
			remotePackages = append(remotePackages, remotePkg)
			remotePackages_byRepository[repositoryPath] = remotePkg
		}
	}

	// Add to 'remotePackages_byImport'
	for _, importPath := range importPaths_array {
		if remotePkg1, exists := remotePackages_byImport[importPath]; exists {
			if remotePkg1 != remotePkg {
				t.Abort(errors.New("import path \"" + importPath + "\" maps to multiple distinct repositories"))
				return
			}
		} else {
			remotePackages_byImport[importPath] = remotePkg
		}
	}
}
