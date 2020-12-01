package aws

import (
	"strings"
	"sync"
)

var currentARN struct {
	value string
	sync.Mutex
}

var currentReqID struct {
	value string
	sync.Mutex
}

// GetARN returns an ARN of the current running function.
// Thread-safe.
func GetARN() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	return currentARN.value
}

// SetARN stores the given ARN.
// Thread-safe.
func SetARN(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	// remove the version if any
	// format: arn:aws:lambda:<region>:<account-id>:function:<function-name>[:<version>]
	if parts := strings.Split(arn, ":"); len(parts) > 7 {
		arn = strings.Join(parts[:7], ":")
	}

	currentARN.value = arn
}

// GetRequestID returns the currently running function request ID.
func GetRequestID() string {
	currentReqID.Lock()
	defer currentReqID.Unlock()

	return currentReqID.value
}

// SetRequestID stores the currently running function request ID.
func SetRequestID(reqID string) {
	currentReqID.Lock()
	defer currentReqID.Unlock()

	currentReqID.value = reqID
}

// FunctionNameFromARN returns the function name from the currently set ARN.
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
