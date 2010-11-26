package main

import (
	"bytes"
	"exp/eval"
	"fmt"
	"io/ioutil"
	"os"
	pathutil "path"
	"strconv"
	"strings"
	"sync"
)

const (
	configFileName = "goam.conf"
	defaultExeName = "a.out"
	testExeName    = "package-test"
)

var currentConfig *config_file_t = nil
var configCurrent_mutex sync.Mutex

// Reads the specified config file
func readConfig(config *config_file_t) os.Error {
	var err os.Error

	configCurrent_mutex.Lock()
	{
		if *flag_debug {
			println("read config:", config.path)
		}

		currentConfig = config

		w := eval.NewWorld()
		defineFunctions(w)
		err = loadAndRunScript(w, config.path)

		currentConfig = nil
	}
	configCurrent_mutex.Unlock()

	return err
}

// Loads and evaluates the specified Go script
func loadAndRunScript(w *eval.World, path string) os.Error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Write(data)
	return runScript(w, path, buf.String())
}

// Runs the specified Go source code in the context of 'w'
func runScript(w *eval.World, path, sourceCode string) os.Error {
	var err os.Error
	var code eval.Code

	code, err = w.Compile(sourceCode)
	if err != nil {
		str := strings.Replace(err.String(), "input", path, 1)
		return os.NewError(str)
	}

	_, err = code.Run()
	if err != nil {
		return err
	}

	return nil
}


func defineFunctions(w *eval.World) {
	{
		var functionSignature func(string)
		funcType, funcValue := eval.FuncFromNativeTyped(wrapper_Package, functionSignature)
		w.DefineVar("Package", funcType, funcValue)
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
		t.Abort(os.NewError("duplicate target package specification"))
		return
	}

	pkg = strings.TrimSpace(pkg)
	if len(pkg) == 0 {
		t.Abort(os.NewError("the target package cannot be an empty string"))
		return
	}

	if *flag_debug {
		println("(read config) target package = \"" + pkg + "\"")
	}
	currentConfig.targetPackage_orEmpty = pkg
}


// Mapping between [the path of an executable] and [the paths of Go files from which to build the executable]
var executable2sources = make(map[string][]string)
var source2executable = make(map[string]string)

// Signature: func Executable(name string, sources ...string)
func wrapper_Executable(t *eval.Thread, in []eval.Value, out []eval.Value) {
	name := in[0].(eval.StringValue).Get(t)
	_sources := in[1].(eval.StringValue).Get(t)

	var err os.Error

	// Check the name, make the name relative to the local root
	{
		name, err = cleanAndCheckPath(t, name)
		if err != nil {
			t.Abort(err)
			return
		}

		_, file := pathutil.Split(name)
		if file == testExeName {
			t.Abort(os.NewError("executables named \"" + name + "\" are used for tests"))
			return
		}

		name = pathutil.Join(currentConfig.parent.path, name)
		if _, alreadyPresent := executable2sources[name]; alreadyPresent {
			t.Abort(os.NewError("duplicate executable \"" + name + "\""))
			return
		}
	}

	var sources []string
	{
		sources = strings.Fields(_sources)
		if len(sources) == 0 {
			t.Abort(os.NewError("empty list of sources"))
			return
		}

		// Check 'sources[i]', make 'sources[i]' relative to the local root
		for i := 0; i < len(sources); i++ {
			source, err := cleanAndCheckPath(t, sources[i])
			if err != nil {
				t.Abort(err)
				return
			}

			source = pathutil.Join(currentConfig.parent.path, source)

			if _, alreadyPresent := source2executable[source]; alreadyPresent {
				t.Abort(os.NewError("cannot associate file \"" + source + "\" with more than one executable"))
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


func cleanAndCheckPath(t *eval.Thread, path string) (string, os.Error) {
	path = pathutil.Clean(path)

	if len(path) == 0 {
		return "", os.NewError("empty path")
	}
	if pathutil.IsAbs(path) {
		return "", os.NewError("path \"" + path + "\" is not a relative path")
	}
	if strings.HasPrefix(path, "..") {
		return "", os.NewError("path \"" + path + "\" is referring the parental directory")
	}

	return path, nil
}


// Set of directories to ignore.
// This is a set, the values of this hash-map have no meaning.
var ignoredDirs = make(map[string]byte)

// Signature: func IgnoreDir(path string)
func wrapper_IgnoreDir(t *eval.Thread, in []eval.Value, out []eval.Value) {
	path := in[0].(eval.StringValue).Get(t)

	var err os.Error
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

// Signature: func DisableGoFmt(name, source string)
func wrapper_DisableGoFmt(t *eval.Thread, in []eval.Value, out []eval.Value) {
	path := in[0].(eval.StringValue).Get(t)

	var err os.Error
	path, err = cleanAndCheckPath(t, path)
	if err != nil {
		t.Abort(err)
		return
	}

	path = pathutil.Join(currentConfig.parent.path, path)

	if _, alreadyPresent := disabledGoFmt[path]; alreadyPresent {
		t.Abort(os.NewError("gofmt already disabled: \"" + path + "\""))
		return
	}

	if *flag_debug {
		println("(read config) disable gofmt \"" + path + "\"")
	}
	disabledGoFmt[path] = 0
}


// Signature: func MinCompilerVersion(version uint)
func wrapper_MinCompilerVersion(t *eval.Thread, in []eval.Value, out []eval.Value) {
	minVersion := in[0].(eval.UintValue).Get(t)

	if *flag_debug {
		println("(read config) Go compiler min version:", minVersion)
	}

	args := []string{goCompiler_exe.name, "-V"}
	stdout, _, err := goCompiler_exe.run(args, /*dir*/ "", /*in*/ "", /*mergeStdoutAndStderr*/ true)
	if err != nil {
		t.Abort(os.NewError("failed to determine Go compiler version: " + err.String()))
		return
	}

	stdout = strings.TrimSpace(stdout)
	var stdout_split []string = strings.Split(stdout, " ", -1)
	if len(stdout_split) < 3 {
		t.Abort(os.NewError("failed to extract [Go compiler version] from string \"" + stdout + "\""))
		return
	}

	version, err := strconv.Atoui(stdout_split[2])
	if err != nil {
		t.Abort(os.NewError("failed to extract [Go compiler version] from string \"" + stdout + "\""))
		return
	}

	if uint64(version) < minVersion {
		msg := fmt.Sprintf("insufficient Go compiler version: %d, minimum required version is %d", version, minVersion)
		t.Abort(os.NewError(msg))
		return
	}
}


// Signature: func InstallPackage()
func wrapper_InstallPackage(t *eval.Thread, in []eval.Value, out []eval.Value) {
	pkg := currentConfig.targetPackage_orEmpty
	if len(pkg) == 0 {
		t.Abort(os.NewError("no target package has been defined"))
		return
	}

	if _, alreadyPresent := installationCommands_packagesByImport[pkg]; alreadyPresent {
		t.Abort(os.NewError("duplicate installation of package \"" + pkg + "\""))
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

	var err os.Error

	// Check the 'srcPath', make it relative to the local root
	{
		srcPath, err = cleanAndCheckPath(t, srcPath)
		if err != nil {
			t.Abort(err)
			return
		}

		_, file := pathutil.Split(srcPath)
		if file == testExeName {
			t.Abort(os.NewError("cannot install: executables named \"" + srcPath + "\" are used for tests"))
			return
		}

		srcPath = pathutil.Join(currentConfig.parent.path, srcPath)
		if _, alreadyPresent := installationCommands_bySrcPath[srcPath]; alreadyPresent {
			t.Abort(os.NewError("duplicate installation of \"" + srcPath + "\""))
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

	var err os.Error

	// Check the 'srcPath', make it relative to the local root
	{
		srcPath, err = cleanAndCheckPath(t, srcPath)
		if err != nil {
			t.Abort(err)
			return
		}

		srcPath = pathutil.Join(currentConfig.parent.path, srcPath)
		if _, alreadyPresent := installationCommands_bySrcPath[srcPath]; alreadyPresent {
			t.Abort(os.NewError("duplicate installation of \"" + srcPath + "\""))
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
		importPaths_array = strings.Split(importPaths, " ", -1)

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
			t.Abort(os.NewError("repository \"" + repositoryPath + "\": empty list of import paths"))
			return
		}

		// Check for duplicated import paths
		if len(importPaths_array) > 1 {
			importPaths_set := make(map[string]byte)
			for _, importPath := range importPaths_array {
				if _, alreadyExists := importPaths_set[importPath]; alreadyExists {
					t.Abort(os.NewError("repository \"" + repositoryPath + "\": duplicate import path \"" + importPath + "\""))
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
	default:
		t.Abort(os.NewError("repository \"" + repositoryPath + "\": \"" + kindString + "\" is not a valid repository type"))
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
			t.Abort(os.NewError("repository \"" + repositoryPath + "\": empty installation command"))
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
				t.Abort(os.NewError("repository \"" + repositoryPath + "\" redefined with a different repository type"))
				return
			}

			if len(remotePkg1.installCmd) != len(installCommand) {
				t.Abort(os.NewError("repository \"" + repositoryPath + "\" redefined with a different installation command"))
				return
			}
			for i := 0; i < len(installCommand); i++ {
				if remotePkg1.installCmd[i] != installCommand[i] {
					t.Abort(os.NewError("repository \"" + repositoryPath + "\" redefined with a different installation command"))
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
				t.Abort(os.NewError("import path \"" + importPath + "\" maps to multiple distinct repositories"))
				return
			}
		} else {
			remotePackages_byImport[importPath] = remotePkg
		}
	}
}
