// +build serverless

package arn

import (
	"strings"
	"sync"
)

var currentARN struct {
	value string
	sync.Mutex
}

// BuildARN returns an ARN of the current running function.
// Thread-safe.
func Get() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	// FIXME(remy): should remove the version if any

	return currentARN.value
}

// Set stores the given ARN.
// Thread-safe.
func Set(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	currentARN.value = arn
}

// FunctionNameFromARN returns the funtion name from the currently set ARN.
// Thread-safe.
func FunctionNameFromARN() string {
	currentARN.Lock()
	defer currentARN.Unlock()
	if currentARN.value == "" {
		return ""
	}

	parts := strings.Split(currentARN.value, ":")
	return parts[len(parts)-1]
}
