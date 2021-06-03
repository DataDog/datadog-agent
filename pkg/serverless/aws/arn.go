// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

const (
	persistedStateFilePath = "/tmp/dd-lambda-extension-cache.json"
	// RegionEnvVar is used to represent the AWS region environment variable name
	RegionEnvVar       = "AWS_REGION"
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
	qualifierEnvVar    = "AWS_LAMBDA_FUNCTION_VERSION"

	traceOriginMetadataKey   = "_dd.origin"
	traceOriginMetadataValue = "lambda"
	computeStatsKey          = "_dd.compute_stats"
	computeStatsValue        = "1"
	functionARNKey           = "function_arn"
	functionNameKey          = "functionname"
	regionKey                = "region"
	awsAccountKey            = "aws_account"
)

type persistedState struct {
	CurrentARN   string
	CurrentReqID string
}

var currentARN struct {
	value     string
	qualifier string
	sync.Mutex
}

var currentReqID struct {
	value string
	sync.Mutex
}

var currentColdStart struct {
	value bool
	sync.Mutex
}

// GetARN returns an ARN of the current running function.
// Thread-safe.
func GetARN() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	return currentARN.value
}

// GetQualifier returns the qualifier for the current running function.
// Thread-safe
func GetQualifier() string {
	currentARN.Lock()
	defer currentARN.Unlock()
	return currentARN.qualifier
}

// GetColdStart returns whether the current invocation is a cold start
// Thread-safe
func GetColdStart() bool {
	currentColdStart.Lock()
	defer currentColdStart.Unlock()
	return currentColdStart.value
}

// SetARN stores the given ARN.
// Thread-safe.
func SetARN(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	arn = strings.ToLower(arn)

	qualifier := ""
	// remove the version if any
	if parts := strings.Split(arn, ":"); len(parts) > 7 {
		arn = strings.Join(parts[:7], ":")
		qualifier = strings.TrimPrefix(parts[7], "$")
	}

	currentARN.value = arn
	currentARN.qualifier = qualifier
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
}

// SetColdStart stores the cold start state of the function
func SetColdStart(coldstart bool) {
	currentColdStart.Lock()
	defer currentColdStart.Unlock()

	currentColdStart.value = coldstart
}

// PersistCurrentStateToFile persists the current state (ARN and Request ID) to a file.
// This allows the state to be restored after the extension restarts.
// Call this function when the extension shuts down.
func PersistCurrentStateToFile() error {
	dataToPersist := persistedState{
		CurrentARN:   GetARN(),
		CurrentReqID: GetRequestID(),
	}

	file, err := json.MarshalIndent(dataToPersist, "", "")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(persistedStateFilePath, file, 0644)
	if err != nil {
		return err
	}
	return nil
}

// RestoreCurrentStateFromFile restores the current state (ARN and Request ID) from a file
// after the extension is restarted. Call this function when the extension starts.
func RestoreCurrentStateFromFile() error {
	file, err := ioutil.ReadFile(persistedStateFilePath)
	if err != nil {
		return err
	}
	var restoredState persistedState
	err = json.Unmarshal(file, &restoredState)
	if err != nil {
		return err
	}
	SetARN(restoredState.CurrentARN)
	SetRequestID(restoredState.CurrentReqID)
	return nil
}

// FetchFunctionARNFromEnv reconstructs the function arn from what's available
// in the environment.
func FetchFunctionARNFromEnv(accountID string) (string, error) {
	partition := "aws"
	region := os.Getenv(RegionEnvVar)
	functionName := os.Getenv(functionNameEnvVar)
	qualifier := os.Getenv(qualifierEnvVar)

	if len(accountID) == 0 || len(region) == 0 || len(functionName) == 0 {
		return "", log.Errorf("Couldn't construct function arn with accountID:%s, region:%s, functionName:%s", accountID, region, functionName)
	}

	if strings.HasPrefix(region, "us-gov") {
		partition = "aws-us-gov"
	}
	if strings.HasPrefix(region, "cn-") {
		partition = "aws-cn"
	}

	baseARN := fmt.Sprintf("arn:%s:lambda:%s:%s:function:%s", partition, region, accountID, functionName)
	if len(qualifier) > 0 && qualifier != "$LATEST" {
		return fmt.Sprintf("%s:%s", baseARN, qualifier), nil
	}
	return baseARN, nil
}

// GetARNTags returns tags associated with the current ARN
func GetARNTags() []string {
	functionARN := GetARN()
	qualifier := GetQualifier()

	parts := strings.Split(functionARN, ":")
	if len(parts) < 6 {
		return []string{
			fmt.Sprintf("function_arn:%s", strings.ToLower(functionARN)),
		}
	}
	region := parts[3]
	accountID := parts[4]
	functionName := strings.ToLower(parts[6])

	tags := []string{
		fmt.Sprintf("region:%s", region),
		fmt.Sprintf("aws_account:%s", accountID),
		fmt.Sprintf("account_id:%s", accountID),
		fmt.Sprintf("functionname:%s", functionName),
		fmt.Sprintf("function_arn:%s", strings.ToLower(functionARN)),
	}
	resource := functionName
	if len(qualifier) > 0 {
		_, err := strconv.ParseUint(qualifier, 10, 64)
		if err == nil {
			tags = append(tags, fmt.Sprintf("executedversion:%s", qualifier))
		}
		resource = fmt.Sprintf("%s:%s", resource, qualifier)
	}
	tags = append(tags, fmt.Sprintf("resource:%s", resource))

	return tags
}

// BuildGlobalTagsMap returns tags associated with the given ARN
func BuildGlobalTagsMap(functionARN string, functionName string, region string, awsAccountID string) map[string]string {
	tags := make(map[string]string)
	tags[traceOriginMetadataKey] = traceOriginMetadataValue
	tags[computeStatsKey] = computeStatsValue
	if functionARN != "" {
		tags[functionARNKey] = functionARN
	}
	tags[functionNameKey] = functionName
	if region != "" {
		tags[regionKey] = region
	}
	if awsAccountID != "" {
		tags[awsAccountKey] = awsAccountID
	}
	return tags
}

// FetchAccountID retrieves the AWS Lambda's account id by calling STS
func FetchAccountID(ctx context.Context, svc stsiface.STSAPI) (string, error) {
	// sts.GetCallerIdentity returns information about the current AWS credentials,
	// (including account ID), and is one of the only AWS API methods that can't be
	// denied via IAM.

	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentityWithContext(ctx, input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				return "", log.Errorf("Couldn't get account ID: %s", aerr.Error())
			}
		}
		return "", log.Errorf("Couldn't get account ID: %s", err.Error())
	}
	return *result.Account, nil
}
