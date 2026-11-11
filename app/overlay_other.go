//go:build !windows

package main

import (
	"errors"
	"time"
)

type Monitor struct {
	Index   int    `json:"index"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	W       int32  `json:"w"`
	H       int32  `json:"h"`
	Primary bool   `json:"primary"`
	Name    string `json:"name"`
}

func setDPIAware() {}
func listMonitors() []Monitor {
	return []Monitor{{Index: 0, W: 1920, H: 1200, Primary: true, Name: "TEST"}}
}
func overlayShow(monIdx int, x, y int32, text string, ttl time.Duration) error {
	return errors.New("on-screen markers are available on Windows only")
}
func overlayClear() {}
