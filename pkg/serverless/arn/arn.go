// +build serverless

package arn

import (
	"sync"
)

var currentARN struct {
	value string
	sync.Mutex
}

// BuildARN returns an ARN of the current running function.
func Get() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	return currentARN.value
}

func Set(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	currentARN.value = arn
}
