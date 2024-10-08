// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EnableStreamLogsRuntimeSetting wraps operations to enable or disable remote config stream logs at runtime.
type EnableStreamLogsRuntimeSetting struct {
	enabled bool
}

// NewEnableStreamLogsRuntimeSetting creates a new EnableStreamLogsRuntimeSetting.
func NewEnableStreamLogsRuntimeSetting() *EnableStreamLogsRuntimeSetting {
	return &EnableStreamLogsRuntimeSetting{
		enabled: false,
	}
}

// Description returns the runtime setting's description.
func (s *EnableStreamLogsRuntimeSetting) Description() string {
	return "Enable/disable remote config streamlogs at runtime. Possible values: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (s *EnableStreamLogsRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting.
func (s *EnableStreamLogsRuntimeSetting) Name() string {
	return "enable_streamlogs"
}

// Get returns the current value of the runtime setting.
func (s *EnableStreamLogsRuntimeSetting) Get(config config.Component) (interface{}, error) {
	return config.Get("logs_config.streaming.enable_streamlogs"), nil
}

// Set changes the value of the runtime setting.
func (s *EnableStreamLogsRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var enable bool

	// Switch cases depends on input from terminal or environment variables
	switch v := v.(type) {
	case bool:
		enable = v
	case string:
		var err error
		if enable, err = strconv.ParseBool(v); err != nil {
			return fmt.Errorf("invalid value type: %s", v)
		}
	default:
		return fmt.Errorf("invalid value type: %T", v)
	}

	config.Set("logs_config.streaming.enable_streamlogs", enable, source)
	log.Debugf("enable_streamlogs is set as: %v", enable)
	return nil
}
