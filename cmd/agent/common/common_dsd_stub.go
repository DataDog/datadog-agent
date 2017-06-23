// +build !dogstatsd

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"errors"
)

func CreateDSD() error {
	return errors.New("Dogstatsd support unavailable in build.")
}

func StopDSD() {
	return
}
