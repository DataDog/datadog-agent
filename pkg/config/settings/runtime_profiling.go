// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/api/util"
)

// ProfilingOpts defines the options used for profiling
type ProfilingOpts struct {
	ProfileMutex         bool
	ProfileMutexFraction int
	ProfileBlocking      bool
	ProfileBlockingRate  int
}

// ExecWithRuntimeProfilingSettings runs the callback func with the given runtime profiling settings
func ExecWithRuntimeProfilingSettings(callback func(), opts ProfilingOpts, settingsClient Client) error {
	if err := util.SetAuthToken(); err != nil {
		return fmt.Errorf("unable to set up authentication token: %v", err)
	}

	prev := make(map[string]interface{})
	defer resetRuntimeProfilingSettings(prev, settingsClient)

	if opts.ProfileMutex && opts.ProfileMutexFraction > 0 {
		old, err := setRuntimeSetting(settingsClient, "runtime_mutex_profile_fraction", opts.ProfileMutexFraction)
		if err != nil {
			return err
		}
		prev["runtime_mutex_profile_fraction"] = old
	}
	if opts.ProfileBlocking && opts.ProfileBlockingRate > 0 {
		old, err := setRuntimeSetting(settingsClient, "runtime_block_profile_rate", opts.ProfileBlockingRate)
		if err != nil {
			return err
		}
		prev["runtime_block_profile_rate"] = old
	}

	callback()

	return nil
}

func setRuntimeSetting(c Client, name string, value int) (interface{}, error) {
	fmt.Fprintln(color.Output, color.BlueString("Setting %s to %v", name, value))

	oldVal, err := c.Get(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get current value of %s: %v", name, err)
	}

	if _, err := c.Set(name, fmt.Sprint(value)); err != nil {
		return nil, fmt.Errorf("failed to set %s to %v: %v", name, value, err)
	}

	return oldVal, nil
}

func resetRuntimeProfilingSettings(prev map[string]interface{}, settingsClient Client) {
	if len(prev) == 0 {
		return
	}

	for name, value := range prev {
		fmt.Fprintln(color.Output, color.BlueString("Restoring %s to %v", name, value))
		if _, err := settingsClient.Set(name, fmt.Sprint(value)); err != nil {
			fmt.Fprintln(color.Output, color.RedString("Failed to restore previous value of %s: %v", name, err))
		}
	}
}
