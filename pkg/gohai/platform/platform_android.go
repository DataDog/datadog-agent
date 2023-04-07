// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build android
// +build android

package platform

func (self *Platform) Collect() (interface{}, error) {
	return nil, nil
}

func Get() (*Platform, []string, error) {
	return nil, nil, nil
}
