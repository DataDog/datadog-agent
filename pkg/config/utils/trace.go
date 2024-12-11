// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetTraceAgentDefaultEnv returns the default env for the trace agent
func GetTraceAgentDefaultEnv(c pkgconfigmodel.Reader) string {
	defaultEnv := ""
	if c.IsSet("apm_config.env") {
		defaultEnv = c.GetString("apm_config.env")
		log.Debugf("Setting DefaultEnv to %q (from apm_config.env)", defaultEnv)
	} else if c.IsSet("env") {
		defaultEnv = c.GetString("env")
		log.Debugf("Setting DefaultEnv to %q (from 'env' config option)", defaultEnv)
	} else {
		for _, tag := range GetConfiguredTags(c, false) {
			if strings.HasPrefix(tag, "env:") {
				defaultEnv = strings.TrimPrefix(tag, "env:")
				log.Debugf("Setting DefaultEnv to %q (from `env:` entry under the 'tags' config option: %q)", defaultEnv, tag)
				return defaultEnv
			}
		}
	}

	return defaultEnv
}
