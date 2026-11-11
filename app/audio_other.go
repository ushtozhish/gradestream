//go:build !windows

package main

import "errors"

var errUnsupported = errors.New("захват звука доступен только в Windows")

func listAudioDevices() ([]AudioDevice, error) { return []AudioDevice{}, nil }
func listAudioApps() ([]AudioApp, error)       { return []AudioApp{}, nil }
func captureDevice(id string, stop, done chan struct{}, ready chan error) {
	close(done)
	ready <- errUnsupported
}
func captureApp(pid uint32, stop, done chan struct{}, ready chan error) {
	close(done)
	ready <- errUnsupported
}
