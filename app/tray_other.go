//go:build !windows

package main

import "log"

func startTray(onShow, onStop, onExit func()) {}
func setTrayState(live bool, viewers int)     {}
func trayNotify(text string)                  { log.Println("NOTIFY:", text) }
func removeTray()                             {}
func showHostWindow() bool                    { return false }
func killBrowser()                            {}
func setProcessAppID()                        {}
func startBranding()                          {}
