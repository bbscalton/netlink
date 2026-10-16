package main

import "syscall"

func bindToDevice(fd int, dev string) error { return syscall.BindToDevice(fd, dev) }
