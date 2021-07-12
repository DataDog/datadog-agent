// +build !windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package aws

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
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

func TestGetAndSetColdstart(t *testing.T) {
	t.Cleanup(resetState)
	SetColdStart(true)

	output := GetColdStart()
	assert.Equal(t, true, output)
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

func TestGetTagsForEnhancedMetrics(t *testing.T) {
	SetARN("arn:aws:lambda:us-east-1:123456789012:function:my-Function:7")
	defer SetARN("")

	generatedTags := GetARNTags()

	assert.Equal(t, generatedTags, []string{
		"region:us-east-1",
		"aws_account:123456789012",
		"account_id:123456789012",
		"functionname:my-function",
		"function_arn:arn:aws:lambda:us-east-1:123456789012:function:my-function",
		"executedversion:7",
		"resource:my-function:7",
	})
}

type mockedSTSAPI struct {
	stsiface.STSAPI
	Resp sts.GetCallerIdentityOutput
}

func (m mockedSTSAPI) GetCallerIdentityWithContext(ctx context.Context, in *sts.GetCallerIdentityInput, options ...request.Option) (*sts.GetCallerIdentityOutput, error) {
	// Only need to return mocked response output
	return &m.Resp, nil
}

func TestFetchFunctionARNFromEnv(t *testing.T) {
	t.Cleanup(resetState)
	os.Setenv(RegionEnvVar, "us-east-1")
	os.Setenv(functionNameEnvVar, "my-Function")
	os.Setenv(qualifierEnvVar, "7")
	arn, err := FetchFunctionARNFromEnv("123456789012")
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-Function:7", arn)
	assert.Nil(t, err)
}

func TestFetchFunctionARNFromEnvGovcloud(t *testing.T) {
	t.Cleanup(resetState)
	os.Setenv(RegionEnvVar, "us-gov-west-1")
	os.Setenv(functionNameEnvVar, "my-Function")
	os.Setenv(qualifierEnvVar, "7")
	arn, err := FetchFunctionARNFromEnv("123456789012")
	assert.Equal(t, "arn:aws-us-gov:lambda:us-gov-west-1:123456789012:function:my-Function:7", arn)
	assert.Nil(t, err)
}

func TestFetchFunctionARNFromEnvChina(t *testing.T) {
	t.Cleanup(resetState)
	os.Setenv(RegionEnvVar, "cn-east-1")
	os.Setenv(functionNameEnvVar, "my-Function")
	os.Setenv(qualifierEnvVar, "7")
	arn, err := FetchFunctionARNFromEnv("123456789012")
	assert.Equal(t, "arn:aws-cn:lambda:cn-east-1:123456789012:function:my-Function:7", arn)
	assert.Nil(t, err)
}

func TestFetchAccountID(t *testing.T) {
	svc := mockedSTSAPI{
		Resp: sts.GetCallerIdentityOutput{
			Account: aws.String("123456789012"),
			Arn:     aws.String(""),
			UserId:  aws.String(""),
		},
	}
	ctx := context.Background()
	result, err := FetchAccountID(ctx, svc)
	assert.Equal(t, "123456789012", result)
	assert.Nil(t, err)
}

func resetState() {
	SetARN("")
	SetRequestID("")
	SetColdStart(false)
	os.Setenv(RegionEnvVar, "")
	os.Setenv(functionNameEnvVar, "")
	os.Setenv(qualifierEnvVar, "")
}

func TestBuildGlobalTagsMap(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	awsAccount := "123456789012"
	functionName := "my-function"
	region := "us-east-1"
	m := BuildGlobalTagsMap(arn, functionName, region, awsAccount)
	assert.Equal(t, len(m), 6)
	assert.Equal(t, region, m["region"])
	assert.Equal(t, functionName, m["functionname"])
	assert.Equal(t, awsAccount, m["aws_account"])
	assert.Equal(t, arn, m["function_arn"])
	assert.Equal(t, "lambda", m["_dd.origin"])
	assert.Equal(t, "1", m["_dd.compute_stats"])
}
