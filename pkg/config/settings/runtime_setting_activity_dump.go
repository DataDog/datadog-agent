// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// MaxDumpSizeConfKey defines the full config key for rate_limiter
	MaxDumpSizeConfKey = "runtime_security_config.activity_dump.max_dump_size"
)

// ActivityDumpRuntimeSetting wraps operations to change activity dumps settings at runtime
type ActivityDumpRuntimeSetting struct {
	ConfigKey string
}

// Description returns the runtime setting's description
func (l ActivityDumpRuntimeSetting) Description() string {
	return "Set/get the corresponding field."
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l ActivityDumpRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (l ActivityDumpRuntimeSetting) Name() string {
	return l.ConfigKey
}

// Get returns the current value of the runtime setting
func (l ActivityDumpRuntimeSetting) Get() (interface{}, error) {
	val := config.SystemProbe.Get(l.ConfigKey)
	return val, nil
}

func (l ActivityDumpRuntimeSetting) setMaxDumpSize(v interface{}) {
	intVar, _ := strconv.Atoi(v.(string))
	config.SystemProbe.Set(l.ConfigKey, intVar)
}

// Set changes the value of the runtime setting
func (l ActivityDumpRuntimeSetting) Set(v interface{}) error {
	val := v.(string)
	log.Infof("ActivityDumpRuntimeSetting Set %s = %s\n", l.ConfigKey, val)

	switch l.ConfigKey {
	case MaxDumpSizeConfKey:
		l.setMaxDumpSize(v)
	default:
		return fmt.Errorf("Field %s does not exist", l.ConfigKey)
	}

	// we trigger a new inventory metadata payload since the configuration was updated by the user.
	inventories.Refresh()
	return nil
}
