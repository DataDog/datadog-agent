// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/proc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	qualifierEnvVar = "AWS_LAMBDA_FUNCTION_VERSION"
	envEnvVar       = "DD_ENV"
	versionEnvVar   = "DD_VERSION"
	serviceEnvVar   = "DD_SERVICE"
	runtimeVar      = "AWS_EXECUTION_ENV"
	memorySizeVar   = "AWS_LAMBDA_FUNCTION_MEMORY_SIZE"

	traceOriginMetadataKey   = "_dd.origin"
	traceOriginMetadataValue = "lambda"
	computeStatsKey          = "_dd.compute_stats"
	computeStatsValue        = "1"
	functionARNKey           = "function_arn"
	functionNameKey          = "functionname"
	regionKey                = "region"
	accountIDKey             = "account_id"
	awsAccountKey            = "aws_account"
	resourceKey              = "resource"
	executedVersionKey       = "executedversion"
	extensionVersionKey      = "dd_extension_version"
	envKey                   = "env"
	versionKey               = "version"
	serviceKey               = "service"
	runtimeKey               = "runtime"
	memorySizeKey            = "memorysize"
	architectureKey          = "architecture"
)

// currentExtensionVersion represents the current version of the Datadog Lambda Extension.
// It is applied to all telemetry as a tag.
// It is replaced at build time with an actual version number.
var currentExtensionVersion = "xxx"

// BuildTagMap builds a map of tag based on the arn and user defined tags
func BuildTagMap(arn string, configTags []string) map[string]string {
	tags := make(map[string]string)

	architecture := ResolveRuntimeArch()

	tags = setIfNotEmpty(tags, architectureKey, architecture)

	tags = setIfNotEmpty(tags, runtimeKey, getRuntime("/proc", "/etc", runtimeVar))
	tags = setIfNotEmpty(tags, memorySizeKey, os.Getenv(memorySizeVar))

	tags = setIfNotEmpty(tags, envKey, os.Getenv(envEnvVar))
	tags = setIfNotEmpty(tags, versionKey, os.Getenv(versionEnvVar))
	tags = setIfNotEmpty(tags, serviceKey, os.Getenv(serviceEnvVar))

	for _, tag := range configTags {
		splitTags := strings.Split(tag, ",")
		for _, singleTag := range splitTags {
			tags = addTag(tags, singleTag)
		}
	}

	tags = setIfNotEmpty(tags, traceOriginMetadataKey, traceOriginMetadataValue)
	tags = setIfNotEmpty(tags, computeStatsKey, computeStatsValue)
	tags = setIfNotEmpty(tags, functionARNKey, arn)
	tags = setIfNotEmpty(tags, extensionVersionKey, currentExtensionVersion)

	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return tags
	}

	tags = setIfNotEmpty(tags, regionKey, parts[3])
	tags = setIfNotEmpty(tags, awsAccountKey, parts[4])
	tags = setIfNotEmpty(tags, accountIDKey, parts[4])
	tags = setIfNotEmpty(tags, functionNameKey, parts[6])
	tags = setIfNotEmpty(tags, resourceKey, parts[6])

	qualifier := os.Getenv(qualifierEnvVar)
	if len(qualifier) > 0 {
		if qualifier != "$LATEST" {
			tags = setIfNotEmpty(tags, resourceKey, fmt.Sprintf("%s:%s", parts[6], qualifier))
			tags = setIfNotEmpty(tags, executedVersionKey, qualifier)
		}
	}

	return tags
}

// BuildTagsFromMap builds an array of tag based on map of tags
func BuildTagsFromMap(tags map[string]string) []string {
	tagsMap := make(map[string]string)
	tagBlackList := []string{traceOriginMetadataKey, computeStatsKey}
	for k, v := range tags {
		tagsMap[k] = v
	}
	for _, blackListKey := range tagBlackList {
		delete(tagsMap, blackListKey)
	}
	tagsArray := make([]string, 0, len(tagsMap))
	for key, value := range tagsMap {
		tagsArray = append(tagsArray, fmt.Sprintf("%s:%s", key, value))
	}
	return tagsArray
}

// BuildTracerTags builds a map of tag from an existing map of tag removing useless tags for traces
func BuildTracerTags(tags map[string]string) map[string]string {
	tagsMap := make(map[string]string)
	tagBlackList := []string{resourceKey}
	for k, v := range tags {
		tagsMap[k] = v
	}
	for _, blackListKey := range tagBlackList {
		delete(tagsMap, blackListKey)
	}
	return tagsMap
}

// AddColdStartTag appends the cold_start tag to existing tags
func AddColdStartTag(tags []string, coldStart bool) []string {
	tags = append(tags, fmt.Sprintf("cold_start:%v", coldStart))
	return tags
}

func setIfNotEmpty(tagMap map[string]string, key string, value string) map[string]string {
	if key != "" && value != "" {
		tagMap[key] = strings.ToLower(value)
	}
	return tagMap
}

func addTag(tagMap map[string]string, tag string) map[string]string {
	extract := strings.Split(tag, ":")
	if len(extract) == 2 {
		tagMap[strings.ToLower(extract[0])] = strings.ToLower(extract[1])
	}
	return tagMap
}

func getRuntimeFromOsReleaseFile(osReleasePath string) string {
	runtime := ""
	bytesRead, err := ioutil.ReadFile(fmt.Sprintf("%s/os-release", osReleasePath))
	if err != nil {
		log.Debug("could not read os-release file")
		return ""
	}
	regExp := regexp.MustCompile(`PRETTY_NAME="Amazon Linux 2"`)
	result := regExp.FindAll(bytesRead, -1)
	if len(result) == 1 {
		runtime = "provided.al2"
	}
	return runtime
}

func getRuntime(procPath string, osReleasePath string, varName string) string {
	value := proc.SearchProcsForEnvVariable(procPath, varName)
	value = strings.Replace(value, "AWS_Lambda_", "", 1)
	if len(value) == 0 {
		value = getRuntimeFromOsReleaseFile(osReleasePath)
	}
	if len(value) == 0 {
		log.Debug("could not find a valid runtime, defaulting to unknown")
		value = "unknown"
	}
	return value
}
