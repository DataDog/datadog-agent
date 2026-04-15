// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"
	"os"
	"reflect"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// ChangeChecker checks the state of a config did not change
// between `NewChangeChecker()` and `HasChanged()`. It is
// designed to be used in `TestMain` function as follow:
//
//	func TestMain(m *testing.M) {
//		checker := setup.NewChangeChecker(setup.Datadog())
//		exit := m.Run()
//		if checker.HasChanged() {
//			os.Exit(1)
//		}
//		os.Exit(exit)
//	}
type ChangeChecker struct {
	config         pkgconfigmodel.Config
	configSettings map[string]interface{}
}

// NewChangeChecker creates a new instance of ChangeChecker that watches the provided config.
func NewChangeChecker(cfg pkgconfigmodel.Config) *ChangeChecker {
	return &ChangeChecker{
		config:         cfg,
		configSettings: cfg.AllSettings(),
	}
}

// HasChanged returns whether the config changed since NewChangeChecker was called.
// If changes are detected this function displays on the standard error what keys changed.
func (c *ChangeChecker) HasChanged() bool {
	allSettingsAfter := c.config.AllSettings()
	stateHasChanged := false
	for k, before := range c.configSettings {
		after := allSettingsAfter[k]
		delete(allSettingsAfter, k)
		if !reflect.DeepEqual(before, after) {
			_, _ = fmt.Fprintf(os.Stderr, "Config change detected: Key:'%s' previous value:'%+v' new value:'%+v'\n", k, before, after)
			stateHasChanged = true
		}
	}
	for k, v := range allSettingsAfter {
		_, _ = fmt.Fprintf(os.Stderr, "Config change detected: Key:'%s' was set to value:'%+v' but it was not restored to its default value\n", k, v)
		stateHasChanged = true
	}
	return stateHasChanged
}
