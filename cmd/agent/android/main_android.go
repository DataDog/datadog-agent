// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build android

//go:generate go run ../../pkg/config/render_config.go agent ../../pkg/config/config_template.yaml ./dist/datadog.yaml

package ddandroid

import (
	"fmt"

	ddapp "github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/pkg/status"
)

func AndroidMain() {
	ddapp.StartAgent()
}

func GetStatus() string {
	ret := ""

	s, err := status.GetStatus()
	if err != nil {
		return fmt.Sprintf("Error getting status %v", err)
	}

	//statusJSON, err := json.Marshal(s)
	//if err != nil {
	//		return fmt.Sprintf("Error marshalling status %v", err)
	//	}
	ret = fmt.Sprintf("Agent (v%s)\n", s["version"])
	/*
		ret += "Runner stats\n"
		checkstats := s["runnerStats"]["Checks"]
		// checkstats is map of name->interface of stats
		switch x := checkstats.(type) {
		case map[string]interface{}:
			for name, _ := range x {
				ret += fmt.Printf("Check: %s\n", name)
			}
		}*/
	return ret
}
