// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package misconfig

import "github.com/DataDog/datadog-agent/pkg/util/log"

// ToLog outputs warnings about common misconfigurations in the logs
func ToLog() {
	for name, check := range checks {
		if err := check(); err != nil {
			log.Warnf("misconfig: %s: %v", name, err)
		}
	}
}

type checkFn func() error

var checks = map[string]checkFn{}

func registerCheck(name string, c checkFn) {
	checks[name] = c
}
