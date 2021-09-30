//go:build (functionaltests && !amd64) || (stresstests && !amd64)
// +build functionaltests,!amd64 stresstests,!amd64

package tests

var supportedSyscalls = map[string]uintptr{}
