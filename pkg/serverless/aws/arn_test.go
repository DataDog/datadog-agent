package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	exampleArn               = "arn:aws:lambda:us-east-1:123456789012:function:my-function:7"
	exampleArnWithoutVersion = "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	exampleFunctionName      = "my-function"
)

func TestGetAndSetARN(t *testing.T) {
	t.Cleanup(resetState)
	SetARN(exampleArn)

	output := GetARN()
	assert.Equal(t, exampleArnWithoutVersion, output)

	cachedArn := getCurrentARNFromCache()
	assert.Equal(t, exampleArnWithoutVersion, cachedArn)

	functionName := FunctionNameFromARN()
	assert.Equal(t, exampleFunctionName, functionName)
}

func TestGetAndSetRequestID(t *testing.T) {
	t.Cleanup(resetState)
	SetRequestID("123")

	output := GetRequestID()
	assert.Equal(t, "123", output)

	cachedRequestID := getCurrentRequestIDFromCache()
	assert.Equal(t, "123", cachedRequestID)
}

func TestRestoreCurrentARNFromCache(t *testing.T) {
	t.Cleanup(resetState)
	// Write ARN directly to the cache for testing
	// After the extension is restarted, the ARN will only be in the cache
	cacheARN(exampleArnWithoutVersion)
	output := GetARN()
	assert.Equal(t, "", output)
	RestoreCurrentARNFromCache()
	output = GetARN()
	assert.Equal(t, exampleArnWithoutVersion, output)
}

func TestRestoreCurrentRequestIDFromCache(t *testing.T) {
	t.Cleanup(resetState)
	// Write Request ID directly to the cache for testing
	// After the extension is restarted, the Request ID will only be in the cache
	cacheRequestID("123")
	output := GetRequestID()
	assert.Equal(t, "", output)
	RestoreCurrentRequestIDFromCache()
	output = GetRequestID()
	assert.Equal(t, "123", output)
}

func resetState() {
	SetARN("")
	SetRequestID("")
}
