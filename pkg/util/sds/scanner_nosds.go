// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sds

package sds

// NewScanner returns a no-op scanner when the Agent is built without the `sds`
// build tag: there is no native library to back a real scanner.
func NewScanner(_ []RuleDefinition) (Scanner, error) {
	return NoOpScanner(), nil
}
