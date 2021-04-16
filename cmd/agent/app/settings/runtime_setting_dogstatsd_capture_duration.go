/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package settings

import (
	"fmt"
	"time"
)

// dsdCaptureDurationRuntimeSetting wraps operations to change the duration, in seconds, of traffic captures
type dsdCaptureDurationRuntimeSetting string

func (l dsdCaptureDurationRuntimeSetting) Description() string {
	return "Enable/disable dogstatsd traffic captures. Possible values are: start, stop"
}

func (l dsdCaptureDurationRuntimeSetting) Hidden() bool {
	return false
}

func (l dsdCaptureDurationRuntimeSetting) Name() string {
	return string(l)
}

func (l dsdCaptureDurationRuntimeSetting) Get() (interface{}, error) {
	// TODO
	return 0, nil
}

func (l dsdCaptureDurationRuntimeSetting) Set(v interface{}) error {
	var err error

	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s.Set: Invalid data type", l)
	}

	_, err = time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("Unsupported type for %s: %v", l, err)
	}

	// TODO
	// common.DSD.Capture.SetDuration(d)

	return nil
}
