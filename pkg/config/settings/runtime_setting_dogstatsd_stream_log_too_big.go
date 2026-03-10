// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type DogstatsdStreamLogTooBigSetting struct{}

// NewDogstatsdStreamLogTooBigSetting creates new setting instance.
func NewDogstatsdStreamLogTooBigSetting() *DogstatsdStreamLogTooBigSetting {
	return &DogstatsdStreamLogTooBigSetting{}
}

// Description returns the runtime setting's description
func (s *DogstatsdStreamLogTooBigSetting) Description() string {
	return "Enable logging too big payloads sent via unix stream socket."
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (s *DogstatsdStreamLogTooBigSetting) Hidden() bool {
	return true
}

// Name returns config key of the setting
func (s *DogstatsdStreamLogTooBigSetting) Name() string {
	return "dogstatsd_stream_log_too_big"
}

// Get the current value
func (s *DogstatsdStreamLogTooBigSetting) Get(config config.Component) (interface{}, error) {
	return config.GetBool(s.Name()), nil
}

// Set the setting to a new value
func (s *DogstatsdStreamLogTooBigSetting) Set(config config.Component, v interface{}, source model.Source) error {
	new, err := GetBool(v)
	if err != nil {
		return err
	}
	config.Set(s.Name(), new, source)
	return nil
}
