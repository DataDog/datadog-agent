// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildDebugString(t *testing.T) {
	t.Setenv("DD_AAA", "aaa")
	t.Setenv("DD_CCC", "ccc")
	t.Setenv("DD_BBB", "bbb")
	t.Setenv("DD_API_KEY", "dontShowIt")
	t.Setenv("hi", "hello")

	res := buildDebugString()
	// cannot check for strict equality as the CI adds some env variables (ie : DD_REPO_BRANCH_NAME)
	assert.Regexp(t, "Datadog extension version : xxx\\|Datadog environment variables: ((DD_)?.*)DD_AAA=aaa\\|((DD_)?.*)DD_API_KEY=\\*\\*\\*\\|((DD_)?.*)DD_BBB=bbb\\|((DD_)?.*)DD_CCC=ccc\\|((DD_)?.*)", res)
}

func TestObfuscatePairIfNeededInvalid(t *testing.T) {
	assert.Empty(t, obfuscatePairIfNeeded("toto", nil))
	assert.Empty(t, obfuscatePairIfNeeded("", nil))
	assert.Empty(t, obfuscatePairIfNeeded("toto=", nil))
}

func TestObfuscatePairIfNeededValid(t *testing.T) {
	envMap := getEnvVariableToObfuscate()
	assert.Equal(t, "toto=tutu", obfuscatePairIfNeeded("toto=tutu", envMap))
	assert.Equal(t, "DD_API_KEY=***", obfuscatePairIfNeeded("DD_API_KEY=secret", envMap))
	assert.Equal(t, "DD_KMS_API_KEY=***", obfuscatePairIfNeeded("DD_KMS_API_KEY=secret", envMap))
	assert.Equal(t, "DD_API_KEY_SECRET_ARN=***", obfuscatePairIfNeeded("DD_API_KEY_SECRET_ARN=secret", envMap))
}
