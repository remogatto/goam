package main

import (
	"os"
)

func mkdirAll(path string, perm uint32) os.Error {
	if *flag_debug {
		println("mkdir-all:", path)
	}
	return os.MkdirAll(path, perm)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
