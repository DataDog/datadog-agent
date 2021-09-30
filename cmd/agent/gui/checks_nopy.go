// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python
// +build !python

package gui

// stub: no python interprter
func getPythonChecks() ([]string, error) {
	return []string{}, nil
}
