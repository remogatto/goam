package main

import (
	"fmt"
	"os"
	pathutil "path"
	"strings"
)


// Enumeration of supported repository kinds
const (
	GITHUB = iota
)

type remote_package_t struct {
	importPaths []string
	repository  repository_t
	installCmd  []string
}

type repository_t interface {
	Kind() int
	KindString() string
	Path() string
	DashboardPath() string
	CloneOrUpdate() (string, bool, os.Error)
}

type repository_github_t struct {
	repositoryPath string
}


// All remote packages defined by configuration files
var remotePackages []*remote_package_t = nil
var remotePackages_byImport = make(map[string]*remote_package_t)
var remotePackages_byRepository = make(map[string]*remote_package_t)


func installAllRemotePackages() os.Error {
	for _, remotePackage := range remotePackages {
		err := remotePackage.Install()
		if err != nil {
			return err
		}
	}

	return nil
}


// ================
// remote_package_t
// ================

func new_remotePackage(importPaths []string, repository repository_t, installCmd []string) *remote_package_t {
	return &remote_package_t{importPaths, repository, installCmd}
}

func (p *remote_package_t) Check() os.Error {
	// Check for meaningless remote-package definitions
	for _, importPath := range p.importPaths {
		if pkg, defined := importPathResolutionTable[importPath]; defined {
			return os.NewError("import path \"" + importPath + "\" maps to library \"" + pkg.lib.path + "\"," +
				" there is no need to define a remote package")
		}
	}

	return nil
}

func (p *remote_package_t) Install() os.Error {
	var err os.Error

	installationRequired := false
	for _, importPath := range p.importPaths {
		_, err = resolvePackage(importPath, /*test*/ false)
		if err != nil {
			installationRequired = true
			break
		}
	}

	if installationRequired {
		fmt.Fprintf(os.Stdout, "Installing remote package \""+p.repository.Path()+"\"\n")

		projectPath, reportToDashboard, err := p.repository.CloneOrUpdate()
		if err != nil {
			return err
		}

		exe := &Executable{name: p.installCmd[0]}
		err = exe.runSimply(p.installCmd, /*dir*/ projectPath, /*dontPrint*/ false)
		if err != nil {
			return err
		}

		// Check that the command actually installed the package
		for _, importPath := range p.importPaths {
			_, err = resolvePackage(importPath, /*test*/ false)
			if err != nil {
				return os.NewError("remote package \"" + p.repository.Path() + "\"" +
					" failed to provide the library \"" + importPath + "\"")
			}
		}

		if reportToDashboard {
			maybeReportToDashboard(p.repository.DashboardPath())
		}
	}

	return nil
}


// ===================
// repository_github_t
// ===================

func new_repository_github(repositoryPath string) *repository_github_t {
	return &repository_github_t{repositoryPath}
}

func (r *repository_github_t) Kind() int {
	return GITHUB
}

func (r *repository_github_t) KindString() string {
	return "GitHub"
}

func (r *repository_github_t) Path() string {
	return r.repositoryPath
}

func (r *repository_github_t) DashboardPath() string {
	return "github.com/" + r.repositoryPath
}

var git_exe = &Executable{
	name: "git",
}

func (r *repository_github_t) CloneOrUpdate() (string, bool, os.Error) {
	var err os.Error

	user, project := pathutil.Split(r.repositoryPath)
	user = pathutil.Clean(user)

	cloneDir := pathutil.Join(remotePkgInstallRoot, "github.com", user)
	projectDir := pathutil.Join(cloneDir, project)

	var reportToDashboard bool

	var alreadyCloned = fileExists(projectDir)
	if !alreadyCloned {
		err = mkdirAll(cloneDir, 0777)
		if err != nil {
			return "", false, err
		}

		// Clone
		args := []string{git_exe.name, "clone", "https://github.com/" + user + "/" + project + ".git"}
		err = git_exe.runSimply(args, cloneDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}

		reportToDashboard = true
	} else {
		// Update
		var args []string
		if *flag_verbose {
			args = []string{git_exe.name, "pull"}
		} else {
			args = []string{git_exe.name, "pull", "-q"}
		}
		err = git_exe.runSimply(args, projectDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}

		reportToDashboard = false
	}

	return projectDir, reportToDashboard, nil
}


// =================
// Utility functions
// =================

func checkRepositoryPath(kind int, path string) os.Error {
	switch kind {
	case GITHUB:
		// Check for "http://" or similar prefixes
		if strings.Contains(path, "://") {
			return os.NewError("invalid GitHub repository path (try removing \"http://\" or similar prefixes)")
		}
		if strings.HasPrefix(path, "github.com") {
			return os.NewError("invalid GitHub repository path (try without the \"github.com\" prefix)")
		}
		if strings.HasSuffix(path, ".git") {
			return os.NewError("invalid GitHub repository path (try without the \".git\" suffix)")
		}

	default:
		panic("invalid kind")
	}

	return nil
}
