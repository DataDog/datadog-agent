// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build android
// +build android

package platform

// Collects the Platform information.
// Returns an object which can be converted to a JSON or an error if nothing could be collected.
// Tries to collect as much information as possible.
func (platform *Platform) Collect() (interface{}, error) {
	return nil, nil
}

// Get returns a Platform struct already initialized, a list of warnings and an error. The method will try to collect as much
// metadata as possible, an error is returned if nothing could be collected. The list of warnings contains errors if
// some metadata could not be collected.
func Get() (*Platform, []string, error) {
	return nil, nil, nil
}
