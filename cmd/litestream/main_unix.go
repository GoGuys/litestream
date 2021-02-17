// +build !windows

package main

func isWindowsService() (bool, error) {
	return false, nil
}

func runWindowsService() error {
	panic("cannot run windows service as unix process")
}
