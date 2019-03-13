// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !cpython,!python

package gui

// stub: no python interprter
func getPythonChecks() ([]string, error) {
	return []string{}, nil
}
