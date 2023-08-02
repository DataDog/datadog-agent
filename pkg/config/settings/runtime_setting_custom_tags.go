// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// CustomTagsRuntimeSetting wraps operations to change custom tags at runtime.
type CustomTagsRuntimeSetting struct {
	Config    config.ConfigReaderWriter
	ConfigKey string
}

// Description returns the runtime setting's description
func (l CustomTagsRuntimeSetting) Description() string {
	return "Set/get the custom tags, valid input is: 'span-name':'custom tags', where custom_tags is an array of string values of tag names separated by a comma"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l CustomTagsRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (l CustomTagsRuntimeSetting) Name() string {
	return "custom_tags"
}

// Get returns the current value of the runtime setting
func (l CustomTagsRuntimeSetting) Get() {
	return
}

// Set changes the value of the runtime setting
func (l CustomTagsRuntimeSetting) Set() {
	return
}
