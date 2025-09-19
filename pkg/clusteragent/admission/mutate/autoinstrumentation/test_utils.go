// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package autoinstrumentation

import (
	"crypto/sha256"
	"fmt"
)

// GenerateTestDigest creates a valid SHA256 digest for testing purposes
func GenerateTestDigest(input string) string {
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("sha256:%x", hash)
}