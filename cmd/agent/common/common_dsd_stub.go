// +build !dogstatsd

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"errors"
)

// CreateDSD a stub in this build
func CreateDSD() error {
	return errors.New("dogstatsd support unavailable in build")
}

// StopDSD a stub in this build
func StopDSD() {
	return
}
