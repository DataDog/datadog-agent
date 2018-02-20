// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !cpython

package gui

// stub: no python interprter
func getPythonChecks() ([]string, error) {
	return []string{}, nil
}
