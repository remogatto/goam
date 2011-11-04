package main

import (
	"errors"
	"io"
	"os"
	pathutil "path"
	"strings"
)

// An installation command
type installation_command_t interface {
	// Performs the installation
	Install(root *dir_t) error

	// Removes the installed files
	Uninstall(root *dir_t) error
}

var installationCommands []installation_command_t = nil
var installationCommands_bySrcPath = make(map[string]installation_command_t)
var installationCommands_packagesByImport = make(map[string]*install_package_t)

// =================
// install_package_t
// =================

type install_package_t struct {
	importPath string
}

func new_installPackage(importPath string) *install_package_t {
	return &install_package_t{importPath}
}

func (i *install_package_t) find() (*package_resolution_t, error) {
	pkg, ok := importPathResolutionTable[i.importPath]
	if !ok {
		return nil, errors.New("unable to install package \"" + i.importPath + "\": no such package")
	}

	return pkg, nil
}

func (i *install_package_t) Install(root *dir_t) error {
	pkg, err := i.find()
	if err != nil {
		return err
	}

	err = pkg.lib.Install(i.importPath)
	if err != nil {
		return err
	}

	return nil
}

func (i *install_package_t) Uninstall(root *dir_t) error {
	pkg, err := i.find()
	if err != nil {
		return err
	}

	err = pkg.lib.Uninstall(i.importPath)
	if err != nil {
		return err
	}

	return nil
}

// ====================
// install_executable_t
// ====================

type install_executable_t struct {
	srcPath string
}

func new_installExecutable(srcPath string) *install_executable_t {
	return &install_executable_t{srcPath}
}

func (i *install_executable_t) find(root *dir_t) (*executable_t, error) {
	var _exe object_t = root.getObject_orNil(strings.Split(i.srcPath, "/"))
	if _exe == nil {
		return nil, errors.New("unable to locate executable \"" + i.srcPath + "\"")
	}

	var exe *executable_t
	var isExe bool
	exe, isExe = _exe.(*executable_t)
	if !isExe {
		return nil, errors.New("\"" + _exe.Path() + "\" is not an executable")
	}

	return exe, nil
}

func (i *install_executable_t) Install(root *dir_t) error {
	exe, err := i.find(root)
	if err != nil {
		return err
	}

	return exe.Install()
}

func (i *install_executable_t) Uninstall(root *dir_t) error {
	exe, err := i.find(root)
	if err != nil {
		return err
	}

	return exe.Uninstall()
}

// =============
// install_dir_t
// =============

type install_dir_t struct {
	srcPath string
	dstPath string
}

func new_installDir(srcPath, dstPath string) *install_dir_t {
	return &install_dir_t{srcPath, dstPath}
}

var cp_exe = &Executable{
	name: "cp",
}

func (i *install_dir_t) Install(root *dir_t) error {
	dstFullPath := pathutil.Join(libInstallRoot, i.dstPath)

	err := mkdirAll(dstFullPath, 0777)
	if err != nil {
		return err
	}

	args := []string{cp_exe.name, "-a", i.srcPath, dstFullPath}
	err = cp_exe.runSimply(args, /*dir*/ "", /*dontPrint*/ false)
	if err != nil {
		return err
	}

	return nil
}

func (i *install_dir_t) Uninstall(root *dir_t) error {
	dstFullPath := pathutil.Join(libInstallRoot, i.dstPath, i.srcPath)

	err := dualWalk(i.srcPath, dstFullPath, uninstaller_t{})
	if err != nil {
		return err
	}

	err = uninstallEmptyDirs(libInstallRoot, pathutil.Join(i.dstPath, i.srcPath))
	if err != nil {
		return err
	}

	return nil
}

// =============
// uninstaller_t
// =============

type uninstaller_t struct{}

func (uninstaller_t) EnterDir(masterPath, slavePath string, masterDir, slave_orNil *os.FileInfo) error {
	return nil
}

func (uninstaller_t) VisitFile(masterPath, slavePath string, master, slave_orNil *os.FileInfo) error {
	if slave_orNil != nil {
		if *flag_debug {
			println("uninstall:", slavePath)
		}

		err := os.Remove(slavePath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (uninstaller_t) LeaveDir(masterPath, slavePath string, masterDir, slave_orNil *os.FileInfo) error {
	if slave_orNil != nil {
		if slave_orNil.IsDirectory() {
			isEmpty, err := isEmptyDir(slavePath)
			if err != nil {
				return err
			}

			if isEmpty {
				if *flag_debug {
					println("uninstall dir:", slavePath)
				}
				err = os.Remove(slavePath)
				if err != nil {
					return err
				}
			}
		} else {
			return errors.New("cannot uninstall \"" + slavePath + "\": not a directory")
		}
	}

	return nil
}

type DualVisitor interface {
	EnterDir(masterPath, slavePath string, masterDir, slave_orNil *os.FileInfo) error
	VisitFile(masterPath, slavePath string, master, slave_orNil *os.FileInfo) error
	LeaveDir(masterPath, slavePath string, masterDir, slave_orNil *os.FileInfo) error
}

func dualWalk(master, slave string, v DualVisitor) error {
	var err error

	master_fileInfo, err := os.Lstat(master)
	if err != nil {
		return err
	}

	slave_fileInfo_orNil, err := os.Lstat(slave)
	if err != nil {
		if slave_fileInfo_orNil != nil {
			return err
		}
	}

	return dualWalk_internal(master, slave, master_fileInfo, slave_fileInfo_orNil, v)
}

func dualWalk_internal(masterPath, slavePath string, master, slave_orNil *os.FileInfo, v DualVisitor) error {
	var err error

	switch {
	case master.IsDirectory():
		err = v.EnterDir(masterPath, slavePath, master, slave_orNil)
		if err != nil {
			return err
		}

		// Re-stat the 'slavePath', becase 'EnterDir' might have created or deleted it
		slave_orNil, err = os.Lstat(slavePath)
		if err != nil {
			if slave_orNil != nil {
				return err
			}
		}

		// Descend into the master directory.
		// Call 'dualWalk_internal(...)' for each entry into master directory.
		{
			var master_entries []os.FileInfo
			{
				f, err := os.Open(masterPath)
				if err != nil {
					return err
				}

				master_entries, err = f.Readdir(-1)
				f.Close()
				if err != nil {
					return err
				}
			}

			if (slave_orNil != nil) && slave_orNil.IsDirectory() {
				// The slave exists and it is a directory.
				// Each entry in the master directory implicates an entry in the slave directory.
				for i, _ := range master_entries {
					master_entry := &master_entries[i]
					master_entryPath := pathutil.Join(masterPath, master_entry.Name)
					slave_entryPath := pathutil.Join(slavePath, master_entry.Name)

					slave_entry_orNil, err := os.Lstat(slave_entryPath)
					if err != nil {
						if slave_entry_orNil != nil {
							return err
						}
					}

					err = dualWalk_internal(master_entryPath, slave_entryPath, master_entry, slave_entry_orNil, v)
					if err != nil {
						return err
					}
				}
			} else {
				// The slave does not exist, or it exists but it is not a directory.
				// It is impossible to descend into the non-existent slave directory.
				for i, _ := range master_entries {
					master_entry := &master_entries[i]
					master_entryPath := pathutil.Join(masterPath, master_entry.Name)
					slave_entryPath := pathutil.Join(slavePath, master_entry.Name)
					err = dualWalk_internal(master_entryPath, slave_entryPath, master_entry, nil, v)
					if err != nil {
						return err
					}
				}
			}
		}

		err = v.LeaveDir(masterPath, slavePath, master, slave_orNil)
		if err != nil {
			return err
		}

	case master.IsRegular():
		err = v.VisitFile(masterPath, slavePath, master, slave_orNil)
		if err != nil {
			return err
		}
	}

	return nil
}

// Remove all empty directories along the relative path.
// Stops at the first non-empty directory, or at the directory 'root'.
func uninstallEmptyDirs(root, relativePath string) error {
	path := pathutil.Join(root, relativePath)

	fileInfo, err := os.Stat(path)
	if err == nil {
		if fileInfo.IsDirectory() {
			isEmpty, err := isEmptyDir(path)
			if err != nil {
				return err
			}

			if isEmpty {
				if *flag_debug {
					println("uninstall dir:", path)
				}
				err = os.Remove(path)
				if err != nil {
					return err
				}

				goto up
			} else {
				// The directory isn't empty. Stop.
				return nil
			}
		} else {
			// Not directory. Stop.
			return nil
		}
	} else {
		// The file at 'path' does not exist.
		goto up
	}

up:
	// Try to recursively remove the upper directory.
	upDir, _ := pathutil.Split(relativePath)
	if strings.HasSuffix(upDir, "/") {
		// Remove the trailing slash
		upDir = upDir[0 : len(upDir)-1]
	}
	if len(upDir) > 0 {
		return uninstallEmptyDirs(root, upDir)
	} else {
		// 'root' has been reached
	}

	return nil
}

func isEmptyDir(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Try to read 1 entry to see if the directory is empty
	entries, err := f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return (len(entries) == 0), nil
}
