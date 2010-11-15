include $(GOROOT)/src/Make.inc

TARG=goam
GOFILES=\
	arch.go\
	config.go\
	dashboard.go\
	exec.go\
	gofmt.go\
	import.go\
	info.go\
	install.go\
	main.go\
	object_dir.go\
	object_go.go\
	object_makefile.go\
	objects.go\
	readdir.go\
	remote.go\
	utils.go

include $(GOROOT)/src/Make.cmd
