// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aws

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
)

const (
	persistedStateFilePath = "/tmp/dd-lambda-extension-cache.json"
	regionEnvVar           = "AWS_REGION"
	functionNameEnvVar     = "AWS_LAMBDA_FUNCTION_NAME"
	aliasEnvVar            = "AWS_LAMBDA_FUNCTION_VERSION"
)

type persistedState struct {
	CurrentARN   string
	CurrentReqID string
}

var currentARN struct {
	value string
	alias string
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

// GetAlias returns the alias for the current running function.
// Thread-safe
func GetAlias() string {
	currentARN.Lock()
	defer currentARN.Unlock()
	return currentARN.alias
}

// GetColdStart returns whether
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

	alias := ""
	// remove the version if any
	if parts := strings.Split(arn, ":"); len(parts) > 7 {
		arn = strings.Join(parts[:7], ":")
		alias = strings.TrimPrefix(parts[7], "$")
	}

	currentARN.value = arn
	currentARN.alias = alias
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

// BuildFunctionARNFromEnv reconstructs the function arn from what's available
// in the environment.
func BuildFunctionARNFromEnv() string {
	region := os.Getenv(regionEnvVar)
	functionName := os.Getenv(functionNameEnvVar)
	alias := os.Getenv(aliasEnvVar)
	accountID := fetchAccountID()
	arnPrefix := "arn:aws"

	if len(accountID) == 0 || len(region) == 0 || len(functionName) == 0 {
		return ""
	}

	if strings.HasPrefix(region, "us-gov") {
		arnPrefix = "arn:aws-us-gov"
	}
	if strings.HasPrefix(region, "cn-") {
		arnPrefix = "arn:aws-cn"
	}

	baseARN := fmt.Sprintf("%s:lambda:%s:%s:function:%s", arnPrefix, region, accountID, functionName)
	if len(alias) > 0 && alias != "$LATEST" {
		return fmt.Sprintf("%s:%s", baseARN, alias)
	}
	return baseARN
}

// GetARNTags returns tags associated with the current ARN
func GetARNTags() []string {
	functionARN := GetARN()
	alias := GetAlias()

	parts := strings.Split(functionARN, ":")
	if len(parts) < 6 {
		return []string{
			fmt.Sprintf("function_arn:%s", strings.ToLower(functionARN)),
		}
	}
	region := parts[3]
	accountID := parts[4]
	functionName := parts[6]

	tags := []string{
		fmt.Sprintf("region:%s", region),
		fmt.Sprintf("aws_account:%s", accountID),
		fmt.Sprintf("account_id:%s", accountID),
		fmt.Sprintf("functionname:%s", functionName),
		fmt.Sprintf("function_arn:%s", strings.ToLower(functionARN)),
	}
	resource := functionName
	if len(alias) > 0 {
		_, err := strconv.ParseUint(alias, 10, 64)
		if err == nil {
			tags = append(tags, fmt.Sprintf("executedversion:%s", alias))
		}
		resource = fmt.Sprintf("%s:%s", resource, alias)
	}
	tags = append(tags, fmt.Sprintf("resource:%s", resource))

	return tags
}

// FetchAccountID retrieves the AWS Lambda's account id by calling STS
func fetchAccountID() string {
	// sts.GetCallerIdentity returns information about the current AWS credentials,
	// (including account ID), and is one of the only AWS API methods that can't be
	// denied via IAM.

	svc := sts.New(session.New())
	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Errorf("Couldn't get account ID: %s", aerr.Error())
			}
		} else {
			log.Errorf("Couldn't get account ID: %s", err.Error())
		}
		return ""
	}
	return *result.Account
}
