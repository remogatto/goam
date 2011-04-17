package main

import (
	pathutil "path"
	"os"
	"strings"
)

func readDir() (*dir_t, os.Error) {
	name := "."

	fileInfo, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !fileInfo.IsDirectory() {
		return nil, os.NewError("not a directory: " + name)
	}

	dir := new_dir(new_entry(name, fileInfo), /*parent_orNil*/ nil)
	err = readDir_internal(dir)
	if err != nil {
		return nil, err
	}

	return dir, nil
}

func readDir_internal(dir *dir_t) os.Error {
	if *flag_debug {
		println("read dir:", dir.path)
	}

	f, err := os.Open(dir.path)
	if err != nil {
		return err
	}

	var list []os.FileInfo
	list, err = f.Readdir(-1)
	f.Close()
	if err != nil {
		return err
	}

	listSize := len(list)

	objects := make([]object_t, listSize)
	subdirs := make([]*dir_t, listSize)

	numObjects := 0
	numSubdirs := 0
	numTemporarySubdirs := 0
	for i := 0; i < listSize; i++ {
		var entry *os.FileInfo = &list[i]

		// Ignore all entries starting with '.'
		if strings.HasPrefix(entry.Name, ".") {
			if *flag_debug {
				println("ignore:", pathutil.Join(dir.path, entry.Name))
			}
			continue
		}

		var object object_t
		var err os.Error

		switch {
		case entry.IsRegular():
			object, err = identifyFile(pathutil.Join(dir.path, entry.Name), entry, dir)

		case entry.IsDirectory():
			object, err = new_dir(new_entry(pathutil.Join(dir.path, entry.Name), entry), dir), nil
		}

		if err != nil {
			return err
		}
		if object != nil {
			objects[numObjects] = object
			numObjects++

			if dir, isDir := object.(*dir_t); isDir {
				subdirs[numSubdirs] = dir
				numSubdirs++
				if dir.isTemporary() {
					numTemporarySubdirs++
				}
			}
		}
	}

	objects = objects[0:numObjects]
	subdirs = subdirs[0:numSubdirs]

	dir.objects = objects

	if dir.parent_orNil == nil {
		// The presence of sub-directories requires a config file or a Makefile
		if ((numSubdirs - numTemporarySubdirs) > 0) && (dir.config_orNil == nil) && (dir.makefile_orNil == nil) {
			return os.NewError("the root directory has one or more user sub-directories," +
				" therefore it requires a " + configFileName + " file or a Makefile")
		}
	}

	// Evaluate the config file (if any)
	if dir.config_orNil != nil {
		err := readConfig(dir.config_orNil)
		if err != nil {
			return err
		}
	}

	// Dive into sub-directories
	for _, subdir := range subdirs {
		if _, ignore := ignoredDirs[pathutil.Clean(subdir.path)]; ignore {
			if *flag_debug {
				println("do not dive into:", subdir.path)
			}
			continue
		}

		err = readDir_internal(subdir)
		if err != nil {
			return err
		}
	}

	return nil
}

func identifyFile(path string, fi *os.FileInfo, parent *dir_t) (object_t, os.Error) {
	var entry entry_t = new_entry(path, fi)

	if strings.HasSuffix(fi.Name, "_test.go") {
		if *flag_debug {
			println("go test:", path)
		}
		return new_go_test(entry, parent), nil
	}

	if strings.HasSuffix(fi.Name, ".go") {
		if *flag_debug {
			println("go source code:", path)
		}
		return new_go_file(entry, parent), nil
	}

	if (len(fi.Name) == len(configFileName)) && (strings.ToLower(fi.Name) == configFileName) {
		if *flag_debug {
			println("config file:", path)
		}
		return new_config_file(entry, parent)
	}

	if fi.Name == "Makefile" {
		if *flag_debug {
			println("makefile:", path)
		}
		return new_makefile(entry, parent)
	}

	if isCompilationUnit(fi.Name) {
		if *flag_debug {
			println("compilation unit:", path)
		}
		return new_compilation_unit(entry, parent), nil
	}

	if strings.HasSuffix(fi.Name, ".a") {
		if *flag_debug {
			println("library:", path)
		}
		return new_library(entry, parent), nil
	}

	if (fi.Permission() & 0100) != 0 {
		if *flag_debug {
			println("executable:", path)
		}
		return new_executable(entry, parent), nil
	}

	return nil, nil
}

func isCompilationUnit(path string) bool {
	ext := pathutil.Ext(path)
	return (ext == ".o") || (ext == ".5") || (ext == ".6") || (ext == ".8")
}
