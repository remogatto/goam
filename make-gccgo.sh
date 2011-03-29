#!/bin/bash

GOFILES="arch.go config.go dashboard.go exec.go gofmt.go import.go info.go install.go main.go object_dir.go object_go.go object_makefile.go objects.go readdir.go remote.go utils.go"

gccgo -O2 -c -o _gccgo_.8 $GOFILES || exit 1
gccgo -o goam.gcc _gccgo_.8
