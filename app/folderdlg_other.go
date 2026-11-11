//go:build !windows

package main

import (
	"errors"
	"os"
)

var errCancelled = errors.New("cancelled")

// Для проверки вне Windows: путь берётся из переменной GS_PICK
func pickFolder(title, start string) (string, error) {
	if p := os.Getenv("GS_PICK"); p != "" {
		return p, nil
	}
	return "", errCancelled
}
