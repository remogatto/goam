package main

import (
	"os"
	pathutil "path"
)


type package_resolution_t struct {
	lib          *library_t
	includePath  *dir_t
}

var importPathResolutionTable = make(map[string]*package_resolution_t)
var importPathResolutionTable_test = make(map[string]*package_resolution_t)

func mapImportPath(importPath string, lib *library_t, includePath *dir_t, test bool) os.Error {
	var table map[string]*package_resolution_t
	if !test {
		table = importPathResolutionTable
	} else {
		table = importPathResolutionTable_test
	}

	if pkg, alreadyMapped := table[importPath]; alreadyMapped {
		if pkg.lib == lib {
			return nil
		} else {
			return os.NewError("package \"" + importPath + "\" has multiple local resolutions: " + pkg.lib.path + ", " + lib.path)
		}
	}

	table[importPath] = &package_resolution_t{
		lib:          lib,
		includePath:  includePath,
	}

	return nil
}

func resolvePackage(importPath string, test bool) (*package_resolution_t, os.Error) {
	var table map[string]*package_resolution_t
	if !test {
		table = importPathResolutionTable
	} else {
		table = importPathResolutionTable_test
	}

	if pkg, ok := table[importPath]; ok {
		// Use "-I dir" or "-L dir" when compiling/linking
		return pkg, nil
	}

	if (importPath == "C") || (importPath == "unsafe") {
		// No need to use "-I dir" or "-L dir"
		return nil, nil
	}

	dir, base := pathutil.Split(importPath)
	if dir == "." {
		dir = ""
	}

	if !fileExists(pathutil.Join(libInstallRoot, dir, base+".a")) {
		return nil, os.NewError("failed to resolve package \"" + importPath + "\"")
	}

	// No need to use "-I dir" or "-L dir"
	return nil, nil
}
