//go:build !darwin

package main

func restartService() error { return errNoService }
