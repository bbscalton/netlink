//go:build !linux

package main

func bindToDevice(fd int, dev string) error { return nil }
