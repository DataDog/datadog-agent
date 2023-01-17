// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/proc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Environment variables for Unified Service Tagging
	envEnvVar     = "DD_ENV"
	versionEnvVar = "DD_VERSION"
	serviceEnvVar = "DD_SERVICE"

	// Environment variables for the Lambda execution environment info
	qualifierEnvVar = "AWS_LAMBDA_FUNCTION_VERSION"
	runtimeVar      = "AWS_EXECUTION_ENV"
	memorySizeVar   = "AWS_LAMBDA_FUNCTION_MEMORY_SIZE"
	InitType        = "AWS_LAMBDA_INITIALIZATION_TYPE"

	// FunctionARNKey is the tag key for a function's arn
	FunctionARNKey = "function_arn"
	// FunctionNameKey is the tag key for a function's name
	FunctionNameKey = "functionname"
	// ExecutedVersionKey is the tag key for a function's executed version
	ExecutedVersionKey = "executedversion"
	// RuntimeKey is the tag key for a function's runtime (e.g node, python)
	RuntimeKey = "runtime"
	// MemorySizeKey is the tag key for a function's allocated memory size
	MemorySizeKey = "memorysize"
	// ArchitectureKey is the tag key for a function's architecture (e.g. x86_64, arm64)
	ArchitectureKey = "architecture"

	// EnvKey is the tag key for a function's env environment variable
	EnvKey = "env"
	// VersionKey is the tag key for a function's version environment variable
	VersionKey = "version"
	// ServiceKey is the tag key for a function's service environment variable
	ServiceKey = "service"

	// SnapStartValue is the Lambda init type env var value indicating SnapStart initialized the function
	SnapStartValue = "snap-start"

	traceOriginMetadataKey   = "_dd.origin"
	traceOriginMetadataValue = "lambda"

	computeStatsKey   = "_dd.compute_stats"
	computeStatsValue = "1"

	extensionVersionKey = "dd_extension_version"

	regionKey     = "region"
	accountIDKey  = "account_id"
	awsAccountKey = "aws_account"
	resourceKey   = "resource"

	// X86LambdaPlatform is for the lambda platform X86_64
	X86LambdaPlatform = "x86_64"
	// ArmLambdaPlatform is for the lambda platform Arm64
	ArmLambdaPlatform = "arm64"
	// AmdLambdaPlatform is for the lambda platform Amd64, which is an extendion of X86_64
	AmdLambdaPlatform = "amd64"
)

// currentExtensionVersion represents the current version of the Datadog Lambda Extension.
// It is applied to all telemetry as a tag.
// It is replaced at build time with an actual version number.
var currentExtensionVersion = "xxx"

// BuildTagMap builds a map of tag based on the arn and user defined tags
func BuildTagMap(arn string, configTags []string) map[string]string {
	tags := make(map[string]string)

	architecture := ResolveRuntimeArch()
	tags = setIfNotEmpty(tags, ArchitectureKey, architecture)

	tags = setIfNotEmpty(tags, RuntimeKey, getRuntime("/proc", "/etc", runtimeVar))

	tags = setIfNotEmpty(tags, MemorySizeKey, os.Getenv(memorySizeVar))

	tags = setIfNotEmpty(tags, EnvKey, os.Getenv(envEnvVar))
	tags = setIfNotEmpty(tags, VersionKey, os.Getenv(versionEnvVar))
	tags = setIfNotEmpty(tags, ServiceKey, os.Getenv(serviceEnvVar))

	for _, tag := range configTags {
		splitTags := strings.Split(tag, ",")
		for _, singleTag := range splitTags {
			tags = addTag(tags, singleTag)
		}
	}

	tags = setIfNotEmpty(tags, traceOriginMetadataKey, traceOriginMetadataValue)
	tags = setIfNotEmpty(tags, computeStatsKey, computeStatsValue)
	tags = setIfNotEmpty(tags, FunctionARNKey, arn)
	tags = setIfNotEmpty(tags, extensionVersionKey, GetExtensionVersion())

	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return tags
	}

	tags = setIfNotEmpty(tags, regionKey, parts[3])
	tags = setIfNotEmpty(tags, awsAccountKey, parts[4])
	tags = setIfNotEmpty(tags, accountIDKey, parts[4])
	tags = setIfNotEmpty(tags, FunctionNameKey, parts[6])
	tags = setIfNotEmpty(tags, resourceKey, parts[6])

	qualifier := os.Getenv(qualifierEnvVar)
	if len(qualifier) > 0 {
		if qualifier != "$LATEST" {
			tags = setIfNotEmpty(tags, resourceKey, fmt.Sprintf("%s:%s", parts[6], qualifier))
			tags = setIfNotEmpty(tags, ExecutedVersionKey, qualifier)
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

// AddInitTypeTag appends the init_type tag to existing tags
func AddInitTypeTag(tags []string) []string {
	initType := os.Getenv(InitType)
	if initType != "" {
		tags = append(tags, fmt.Sprintf("init_type:%v", initType))
	}
	return tags
}

// GetExtensionVersion returns the extension version which is fed at build time
func GetExtensionVersion() string {
	return currentExtensionVersion
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
	bytesRead, err := os.ReadFile(fmt.Sprintf("%s/os-release", osReleasePath))
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
	foundRuntimes := proc.SearchProcsForEnvVariable(procPath, varName)
	runtime := cleanRuntimes(foundRuntimes)
	runtime = strings.Replace(runtime, "AWS_Lambda_", "", 1)
	if len(runtime) == 0 {
		runtime = getRuntimeFromOsReleaseFile(osReleasePath)
	}
	if len(runtime) == 0 {
		log.Debug("could not find a valid runtime, defaulting to unknown")
		runtime = "unknown"
	}
	return runtime
}

func cleanRuntimes(runtimes []string) string {
	filtered := []string{}
	for i := range runtimes {
		if runtimes[i] != "AWS_Lambda_rapid" {
			filtered = append(filtered, runtimes[i])
		}
	}
	if len(filtered) != 1 {
		log.Debug("could not find a unique value for runtime")
		return ""
	}
	return filtered[0]
}
