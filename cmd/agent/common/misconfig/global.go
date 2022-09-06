// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package misconfig

import "github.com/DataDog/datadog-agent/pkg/util/log"

// ToLog outputs warnings about common misconfigurations in the logs
func ToLog(agent string) {
	for _, check := range checks {
		if check.Agent == agent {
			if err := check.Run(); err != nil {
				log.Warnf("misconfig: %s: %v", agent, err)
			}
		}
	}
}

type checkFn func() error
type check struct {
	Agent string
	Run   checkFn
}

var checks = map[string]check{}

// nolint: deadcode, unused
func registerCheck(name string, c checkFn) {
	checks[name] = check{Agent: name, Run: c}
}
