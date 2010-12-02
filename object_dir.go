package main

import (
	"container/vector"
	"io"
	"os"
	pathutil "path"
)

// Represents a directory
type dir_t struct {
	entry_t
	parent_orNil   *dir_t
	config_orNil   *config_file_t
	makefile_orNil *makefile_t
	numTestFiles   uint
	objects        []object_t
}


func new_dir(entry entry_t, parent_orNil *dir_t) *dir_t {
	d := &dir_t{
		entry_t:        entry,
		parent_orNil:   parent_orNil,
		config_orNil:   nil,
		makefile_orNil: nil,
		numTestFiles:   0,
		objects:        nil,
	}
	newObjects[d] = 0
	return d
}

func (d *dir_t) root() *dir_t {
	if d.parent_orNil != nil {
		return d.parent_orNil.root()
	}

	return d
}

func (d *dir_t) getObject_orNil(path []string) object_t {
	switch {
	case len(path) == 0:
		return d

	case len(path) == 1:
		// Return the object with name 'path[0]'
		for _, object := range d.objects {
			if object.Name() == path[0] {
				return object
			}
		}

	default:
		// Continue in sub-directory whose name is 'path[0]'
		for _, object := range d.objects {
			if subdir, isDir := object.(*dir_t); isDir {
				if subdir.name == path[0] {
					return subdir.getObject_orNil(path[1:])
				}
			}
		}
	}

	// Not found
	return nil
}

func (d *dir_t) getOrCreateSubDir(name string) *dir_t {
	if (name == "") || (name == ".") {
		return d
	}

	// Try to find a sub-directory named 'name'
	for _, object := range d.objects {
		if subdir, isDir := object.(*dir_t); isDir {
			if subdir.name == name {
				return subdir
			}
		}
	}

	// Append [a new directory named 'name'] to 'd.objects'
	path := pathutil.Join(d.path, name)
	newSubDir := new_dir(new_entry_from_path(name, path), /*parent_orNil*/ d)
	d.add(newSubDir)
	return newSubDir
}

func (d *dir_t) getOrCreateSubDirs(names []string) *dir_t {
	for _, name := range names {
		d = d.getOrCreateSubDir(name)
	}

	return d
}

func (d *dir_t) mkdir_ifDoesNotExist() os.Error {
	if d.parent_orNil != nil {
		err := d.parent_orNil.mkdir_ifDoesNotExist()
		if err != nil {
			return err
		}
	}

	if !d.exists {
		if *flag_debug {
			println("mkdir:", d.path)
		}

		err := os.Mkdir(d.path, 0777)
		if err != nil {
			return err
		}

		d.UpdateFileInfo()
	}

	return nil
}

// Adds the object to 'd.objects'
func (d *dir_t) add(object object_t) {
	d.objects = append(d.objects, object)
}

func (d *dir_t) getOrCreate_goTestMain(fileName, importPath string, testDir *dir_t) (*go_testMain_t, os.Error) {
	var t *go_testMain_t

	path := pathutil.Join(d.path, fileName)

	var _t object_t = d.getObject_orNil([]string{fileName})
	if _t == nil {
		// Create a new instance of 'go_testMain_t'
		t = new_go_testMain(new_nonexistent_entry(fileName, path), /*parent*/ d, importPath)
		d.add(t)
	} else {
		var isTest bool
		t, isTest = _t.(*go_testMain_t)
		if !isTest {
			if goFile, isGoFile := _t.(*go_file_t); isGoFile {
				// Transform the object's type: go_file_t --> go_testMain_t
				d.removeObject(goFile)
				t = new_go_testMain(goFile.entry_t, /*parent*/ d, importPath)
				d.add(t)
			} else {
				return nil, os.NewError("file \"" + _t.Path() + "\" has an invalid type")
			}
		}
	}

	err := t.setImportPath(importPath)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (d *dir_t) getOrCreate_compilationUnit(name string) (*compilation_unit_t, os.Error) {
	var compilationUnit *compilation_unit_t

	var _compilationUnit object_t = d.getObject_orNil([]string{name})
	if _compilationUnit == nil {
		// Create a new instance of 'compilation_unit_t'
		path := pathutil.Join(d.path, name)
		compilationUnit = new_compilation_unit(new_nonexistent_entry(name, path), /*parent*/ d)
		d.add(compilationUnit)
	} else {
		var isCompilationUnit bool
		compilationUnit, isCompilationUnit = _compilationUnit.(*compilation_unit_t)
		if !isCompilationUnit {
			return nil, os.NewError("file \"" + _compilationUnit.Path() + "\" was expected to be a " + o_ext + " file")
		}
	}

	return compilationUnit, nil
}

func (d *dir_t) getOrCreate_library(name string) (*library_t, os.Error) {
	var lib *library_t

	var _lib object_t = d.getObject_orNil([]string{name})
	if _lib == nil {
		// Create a new instance of 'library_t'
		path := pathutil.Join(d.path, name)
		lib = new_library(new_nonexistent_entry(name, path), /*parent*/ d)
		d.add(lib)
	} else {
		var isLib bool
		lib, isLib = _lib.(*library_t)
		if !isLib {
			return nil, os.NewError("file \"" + _lib.Path() + "\" was expected to be a library")
		}
	}

	return lib, nil
}

func (d *dir_t) getOrCreate_dynLibrary(name string) (*dyn_library_t, os.Error) {
	var lib *dyn_library_t

	var _lib object_t = d.getObject_orNil([]string{name})
	if _lib == nil {
		// Create a new instance of 'library_t'
		path := pathutil.Join(d.path, name)
		lib = new_dyn_library(new_nonexistent_entry(name, path), /*parent*/ d)
		d.add(lib)
	} else {
		var isDynLib bool
		lib, isDynLib = _lib.(*dyn_library_t)
		if !isDynLib {
			return nil, os.NewError("file \"" + _lib.Path() + "\" was expected to be a dynamic library")
		}
	}

	return lib, nil
}

func (d *dir_t) getOrCreate_executable(name string) (*executable_t, os.Error) {
	var exe *executable_t

	var _exe object_t = d.getObject_orNil([]string{name})
	if _exe == nil {
		// Create a new instance of 'executable_t'
		path := pathutil.Join(d.path, name)
		exe = new_executable(new_nonexistent_entry(name, path), /*parent*/ d)
		d.add(exe)
	} else {
		var isExe bool
		exe, isExe = _exe.(*executable_t)
		if !isExe {
			return nil, os.NewError("file \"" + _exe.Path() + "\" was expected to be an executable")
		}
	}

	return exe, nil
}

func (d *dir_t) removeObject(o object_t) {
	// Optional: remove from 'newObjects'
	if _, contains := newObjects[o]; contains {
		newObjects[o] = 0, false
	}

	// Remove from 'd.objects'
	for i, object := range d.objects {
		if o == object {
			copy(d.objects[i:], d.objects[i+1:])
			d.objects = d.objects[0 : len(d.objects)-1]
			return
		}
	}

	panic("no such object")
}


func (d *dir_t) UpdateFileSystemModel() {
	d.UpdateFileInfo()
	for _, object := range d.objects {
		object.UpdateFileSystemModel()
	}
}

func (d *dir_t) InferObjects(updateTests bool) os.Error {
	return nil
}

func (d *dir_t) PrintDependencies(w io.Writer) {
	for _, object := range d.objects {
		object.PrintDependencies(w)
	}
}

func (d *dir_t) shouldRemove() bool {
	switch d.name {
	case "_obj", "_test":
		return true
	}

	if d.parent_orNil != nil {
		return d.parent_orNil.shouldRemove()
	}

	return false
}

func (d *dir_t) Info(info *info_t) {
	for _, object := range d.objects {
		object.Info(info)
	}
}

func (d *dir_t) Make() os.Error {
	var err os.Error

	if d.name == "_test" {
		return nil
	}

	err = d.mkdir_ifDoesNotExist()
	if err != nil {
		return err
	}

	haveMakefile := (d.makefile_orNil != nil)
	if haveMakefile {
		// Execute "make"
		err = d.makefile_orNil.Make()
		if err != nil {
			return err
		}
	} else {
		// Make all objects
		for _, object := range d.objects {
			err = object.Make()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *dir_t) MakeTests() os.Error {
	var err os.Error

	err = d.mkdir_ifDoesNotExist()
	if err != nil {
		return err
	}

	haveMakefile := (d.makefile_orNil != nil)
	if haveMakefile {
		if d.numTestFiles > 0 {
			// Execute "make test"
			err = d.makefile_orNil.MakeTests()
			if err != nil {
				return err
			}
		}
	} else {
		// Call 'MakeTests()' on all objects
		for _, object := range d.objects {
			err = object.MakeTests()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *dir_t) RunTests(testPattern, benchPattern string) os.Error {
	var err os.Error

	haveMakefile := (d.makefile_orNil != nil)
	if haveMakefile {
		if d.numTestFiles > 0 {
			// Execute "make test"
			err = d.makefile_orNil.RunTests(testPattern, benchPattern)
			if err != nil {
				return err
			}
		}
	} else {
		// Call 'RunTests()' on all objects
		for _, object := range d.objects {
			err = object.RunTests(testPattern, benchPattern)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *dir_t) Clean() os.Error {
	var err os.Error

	if d.exists {
		if d.makefile_orNil != nil {
			// Execute "make clean"
			err = d.makefile_orNil.Clean()
			if err != nil {
				return err
			}
		}
	}

	if d.exists {
		for _, object := range d.objects {
			makefile, isMakefile := object.(*makefile_t)
			if isMakefile && (makefile == d.makefile_orNil) {
				continue
			}

			err = object.Clean()
			if err != nil {
				return err
			}
		}

		if d.shouldRemove() {
			if *flag_debug {
				println("remove dir:", d.path)
			}
			err = os.Remove(d.path)
			if err != nil {
				return err
			}

			d.exists = false
		}
	}

	return nil
}

func (d *dir_t) GoFmt(files *vector.StringVector) os.Error {
	for _, object := range d.objects {
		err := object.GoFmt(files)
		if err != nil {
			return err
		}
	}

	return nil
}
