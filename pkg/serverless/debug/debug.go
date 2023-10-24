// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package debug holds logic around debug information in the Lambda Extension
package debug

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ddPrefix = "DD_"

// OutputDatadogEnvVariablesForDebugging outputs the Datadog environment variables for debugging purposes
func OutputDatadogEnvVariablesForDebugging() {
	log.Debug(buildDebugString())
}

func buildDebugString() string {
	var sb strings.Builder
	envMap := getEnvVariableToObfuscate()
	sb.WriteString("Datadog extension version : ")
	sb.WriteString(tags.GetExtensionVersion())
	sb.WriteString("|")
	sb.WriteString("Datadog environment variables: ")
	allEnv := os.Environ()
	sort.Strings(allEnv)
	for _, pair := range allEnv {
		if strings.HasPrefix(pair, ddPrefix) {
			sb.WriteString(obfuscatePairIfNeeded(pair, envMap))
			sb.WriteString("|")
		}
	}
	return sb.String()
}

func obfuscatePairIfNeeded(pair string, envMap map[string]bool) string {
	obfuscated := ""
	tokens := strings.Split(pair, "=")
	if len(tokens) == 2 && len(tokens[1]) > 0 {
		if envMap[tokens[0]] {
			obfuscated = fmt.Sprintf("%s=***", tokens[0])
		} else {
			obfuscated = pair
		}

	}
	return obfuscated
}

func getEnvVariableToObfuscate() map[string]bool {
	return map[string]bool{
		"DD_KMS_API_KEY":        true,
		"DD_API_KEY_SECRET_ARN": true,
		"DD_API_KEY":            true,
	}
}
