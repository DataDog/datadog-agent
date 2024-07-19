// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config defines the configuration of the agent
package config

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetLogLevel changes the log level of the Datadog agent.
// It takes a `level` string representing the desired log level and a `source` model.Source indicating the desired subconfiguration to update.
// It returns an error if the log level is invalid or if there is an error setting the log level.
func SetLogLevel(level string, source model.Source) error {
	seelogLogLevel, err := log.ValidateLogLevel(level)
	if err != nil {
		return err
	}
	Datadog().Set("log_level", seelogLogLevel, source)
	return nil
}
