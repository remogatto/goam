# GOAM - A build tool for Go projects

The GOAM build tool is intended for use with medium-size projects implemented
in the [Go programming language](http://golang.org).

For further information examine the "doc" and "examples" directories.

Alternative build tools can be found at
[Go dashboard](http://godashboard.appspot.com/project).


# Features

## Configuration
* Small mostly-declarative configuration files
* Configuration file contains Go source code
* No configuration file is required in the presence of a standard Go Makefile

## Project structure
* Information can be displayed without building the project
* Support for multiple executables in one directory
* Libraries can be private to a project
* Installation and uninstallation
* Download and installation of dependencies (only supports GitHub right now)

## Integration with other tools
* Makefile support
* CGO support (but only via Makefile)
* gotest support
* gofmt support


# Sample projects

* [GoSpeccy](https://github.com/remogatto/gospeccy)


# Installation requirements

* [go-eval](https://bitbucket.org/binet/go-eval)


# Compatibility notes

* The Git branch "master" is in sync with weekly Go releases
* The Git branch "release" is in sync with stable Go releases
* The Git branch "gcc" is in sync with GCC releases
