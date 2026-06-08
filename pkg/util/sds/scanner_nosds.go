// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sds

//nolint:revive
package sds

// Reconfigure is a no-op when the Agent is built without the `sds` build tag:
// there is no scanner to reconfigure.
func Reconfigure(_ []RuleDefinition) error {
	return nil
}

// Scan is a no-op when the Agent is built without the `sds` build tag: it
// returns no matches.
func Scan(_ []byte) ([]Match, error) {
	return nil, nil
}

// ScanMap is a no-op when the Agent is built without the `sds` build tag: it
// returns no matches and leaves the event untouched.
func ScanMap(_ map[string]interface{}) ([]Match, error) {
	return nil, nil
}
