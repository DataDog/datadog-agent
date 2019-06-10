// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build android

package ddandroid

import (
	"fmt"
	"log"
	"strings"

	ddapp "github.com/StackVista/stackstate-agent/cmd/agent/app"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/status"
	"github.com/StackVista/stackstate-agent/pkg/util/androidasset"
)

func AndroidMain(apikey string, hostname string, tags string) {
	overrides := make(map[string]interface{})
	if len(apikey) != 0 {
		overrides["api_key"] = apikey
	}
	if len(hostname) != 0 {
		overrides["hostname"] = hostname
	}
	if len(tags) != 0 {
		overrides["tags"] = strings.Split(tags, ",")
	}
	//readAsset("android.yaml")
	if _, err := androidasset.ReadFile("stackstate.yaml"); err != nil {
		log.Printf("Failed to read datadog yaml asset %v", err)
	} else {
		log.Printf("Read datadog.yaml asset")
	}

	// read the android-specific config in `assets`, which allows us
	// to override config rather than using environment variables
	config.Datadog.SetConfigFile("stackstate.yaml")
	config.SetOverrides(overrides)

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
