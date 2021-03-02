// +build !windows

package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	exampleArn               = "arn:aws:lambda:us-east-1:123456789012:function:my-function:7"
	exampleArnWithoutVersion = "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	exampleFunctionName      = "my-function"
	exampleRequestID         = "123"
)

func TestGetAndSetARN(t *testing.T) {
	t.Cleanup(resetState)
	SetARN(exampleArn)

	output := GetARN()
	assert.Equal(t, exampleArnWithoutVersion, output)

	functionName := FunctionNameFromARN()
	assert.Equal(t, exampleFunctionName, functionName)
}

func TestGetAndSetRequestID(t *testing.T) {
	t.Cleanup(resetState)
	SetRequestID(exampleRequestID)

	output := GetRequestID()
	assert.Equal(t, exampleRequestID, output)
}

func TestPersistAndRestoreCurrentState(t *testing.T) {
	t.Cleanup(resetState)
	SetARN(exampleArn)
	SetRequestID(exampleRequestID)
	PersistCurrentStateToFile()

	SetARN("")
	SetRequestID("")
	output := GetARN()
	assert.Equal(t, "", output)
	output = GetRequestID()
	assert.Equal(t, "", output)

	err := RestoreCurrentStateFromFile()
	assert.Equal(t, err, nil)
	output = GetARN()
	assert.Equal(t, exampleArnWithoutVersion, output)
	output = GetRequestID()
	assert.Equal(t, exampleRequestID, output)
}

func resetState() {
	SetARN("")
	SetRequestID("")
}
