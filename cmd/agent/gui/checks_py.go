// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package gui

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
)

func getPythonChecks() ([]string, error) {
	pyChecks := []string{}

	// The integration list includes JMX integrations, they ship as wheels too.
	// JMX wheels just contain sample configs, but they do ship.
	integrations, err := py.GetPythonIntegrationList()
	if err != nil {
		return []string{}, err
	}

	for _, integration := range integrations {
		if _, ok := check.JMXChecks[integration]; !ok {
			pyChecks = append(pyChecks, integration)
		}
	}

	return pyChecks, nil
}
