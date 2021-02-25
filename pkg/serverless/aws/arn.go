package aws

import (
	"io/ioutil"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cachedARNFilePath       = "/tmp/dd-lambda-extension-function-arn-cache"
	cachedRequestIDFilePath = "/tmp/dd-lambda-extension-request-id-cache"
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

	arn = strings.ToLower(arn)

	// remove the version if any
	// format: arn:aws:lambda:<region>:<account-id>:function:<function-name>[:<version>]
	if parts := strings.Split(arn, ":"); len(parts) > 7 {
		arn = strings.Join(parts[:7], ":")
	}

	currentARN.value = arn
	cacheARN(arn)
}

// cacheARN writes the ARN to a file in the /tmp directory so we can restore it if the extension process is restarted
func cacheARN(arn string) {
	data := []byte(arn)
	err := ioutil.WriteFile(cachedARNFilePath, data, 0644)
	if err != nil {
		log.Error("Error writing ARN to cache")
	}
}

// FunctionNameFromARN returns the function name from the currently set ARN.
// Thread-safe.
func FunctionNameFromARN() string {
	arn := GetARN()
	parts := strings.Split(arn, ":")
	return parts[len(parts)-1]
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
	cacheRequestID(reqID)
}

// cacheRequestID writes the Request ID to a file in the /tmp directory so we can restore it if the extension process is restarted
func cacheRequestID(reqID string) {
	data := []byte(reqID)
	err := ioutil.WriteFile(cachedRequestIDFilePath, data, 0644)
	if err != nil {
		log.Error("Error writing request ID to cache")
	}
}

// RestoreCurrentARNFromCache sets the current ARN to the value cached as a file in case the extension process was restarted
func RestoreCurrentARNFromCache() {
	SetARN(getCurrentARNFromCache())
}

// getCurrentARNFromCache retrieves the cached current ARN
func getCurrentARNFromCache() string {
	data, err := ioutil.ReadFile(cachedARNFilePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// RestoreCurrentRequestIDFromCache sets the current request ID to the value cached as a file in case the extension process was restarted
func RestoreCurrentRequestIDFromCache() {
	SetRequestID(getCurrentRequestIDFromCache())
}

// getCurrentRequestIDFromCache retrieves the cached current request ID
func getCurrentRequestIDFromCache() string {
	data, err := ioutil.ReadFile(cachedRequestIDFilePath)
	if err != nil {
		return ""
	}
	return string(data)
}
