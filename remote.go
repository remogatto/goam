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
	BITBUCKET
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

type repository_bitbucket_t struct {
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

	exe := git_exe

	var alreadyCloned = fileExists(projectDir)
	if !alreadyCloned {
		err = mkdirAll(cloneDir, 0777)
		if err != nil {
			return "", false, err
		}

		// Clone
		args := []string{exe.name, "clone", "https://github.com/" + user + "/" + project + ".git"}
		err = exe.runSimply(args, cloneDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}
	} else {
		// Download changes and update local files
		var args []string
		if *flag_verbose {
			args = []string{exe.name, "pull"}
		} else {
			args = []string{exe.name, "pull", "-q"}
		}
		err = exe.runSimply(args, projectDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}
	}

	return projectDir, true, nil
}


// ======================
// repository_bitbucket_t
// ======================

func new_repository_bitbucket(repositoryPath string) *repository_bitbucket_t {
	return &repository_bitbucket_t{repositoryPath}
}

func (r *repository_bitbucket_t) Kind() int {
	return BITBUCKET
}

func (r *repository_bitbucket_t) KindString() string {
	return "BitBucket"
}

func (r *repository_bitbucket_t) Path() string {
	return r.repositoryPath
}

func (r *repository_bitbucket_t) DashboardPath() string {
	return "bitbucket.org/" + r.repositoryPath
}

var hg_exe = &Executable{
	name: "hg",
}

func (r *repository_bitbucket_t) CloneOrUpdate() (string, bool, os.Error) {
	var err os.Error

	user, project := pathutil.Split(r.repositoryPath)
	user = pathutil.Clean(user)

	cloneDir := pathutil.Join(remotePkgInstallRoot, "bitbucket.org", user)
	projectDir := pathutil.Join(cloneDir, project)

	exe := hg_exe

	var alreadyCloned = fileExists(projectDir)
	if !alreadyCloned {
		err = mkdirAll(cloneDir, 0777)
		if err != nil {
			return "", false, err
		}

		// Clone
		args := []string{exe.name, "clone", "https://bitbucket.org/" + user + "/" + project}
		err = exe.runSimply(args, cloneDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}
	} else {
		// Download changes
		var args []string
		if *flag_verbose {
			args = []string{exe.name, "pull"}
		} else {
			args = []string{exe.name, "pull", "-q"}
		}
		err = exe.runSimply(args, projectDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}

		// Update local files
		if *flag_verbose {
			args = []string{exe.name, "update"}
		} else {
			args = []string{exe.name, "update", "-q"}
		}
		err = exe.runSimply(args, projectDir, /*dontPrint*/ false)
		if err != nil {
			return "", false, err
		}
	}

	return projectDir, true, nil
}


// =================
// Utility functions
// =================

func checkRepositoryPath(kind int, path string) os.Error {
	switch kind {
	case GITHUB:
		if strings.Contains(path, "://") {
			return os.NewError("invalid GitHub repository path (try removing \"http://\" or similar prefixes)")
		}
		if strings.HasPrefix(path, "github.com") {
			return os.NewError("invalid GitHub repository path (try without the \"github.com\" prefix)")
		}
		if strings.HasSuffix(path, ".git") {
			return os.NewError("invalid GitHub repository path (try without the \".git\" suffix)")
		}

	case BITBUCKET:
		if strings.Contains(path, "://") {
			return os.NewError("invalid BitBucket repository path (try removing \"http://\" or similar prefixes)")
		}
		if strings.HasPrefix(path, "bitbucket.org") {
			return os.NewError("invalid BitBucket repository path (try without the \"bitbucket.org\" prefix)")
		}

	default:
		panic("invalid kind")
	}

	return nil
}
