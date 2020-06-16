// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// SuiteMeta contains metadata for a compliance suite
type SuiteMeta struct {
	Name      string   `yaml:"name,omitempty"`
	Framework string   `yaml:"framework,omitempty"`
	Version   string   `yaml:"version,omitempty"`
	Tags      []string `yaml:"tags,omitempty"`
}

// Suite represents a set of compliance checks reporting events
type Suite struct {
	Meta  SuiteMeta `yaml:",inline"`
	Rules []Rule    `yaml:"rules,omitempty"`
}

// ParseSuite loads a single compliance suite
func ParseSuite(config string) (*Suite, error) {
	f, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}
	s := &Suite{}
	err = yaml.Unmarshal(f, s)
	if err != nil {
		return nil, err
	}
	return s, nil
}
