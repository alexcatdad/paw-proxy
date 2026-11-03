//go:build !darwin && !linux

package main

func restartService() error { return errNoService }
