// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"errors"
	"io/ioutil"

	"github.com/Masterminds/semver"
	"gopkg.in/yaml.v2"
)

const versionConstraint = "<= 1.0.0"

// ErrUnsupportedSchemaVersion is returned for a schema version not supported by this version of the agent
var ErrUnsupportedSchemaVersion = errors.New("schema version not supported")

// SuiteSchema defines versioning for a compliance suite
type SuiteSchema struct {
	Version string `yaml:"version"`
}

// SuiteMeta contains metadata for a compliance suite
type SuiteMeta struct {
	Schema    SuiteSchema `yaml:"schema,omitempty"`
	Name      string      `yaml:"name,omitempty"`
	Framework string      `yaml:"framework,omitempty"`
	Version   string      `yaml:"version,omitempty"`
	Tags      []string    `yaml:"tags,omitempty"`
	Source    string      `yaml:"-"`
}

// Suite represents a set of compliance checks reporting events
type Suite struct {
	Meta      SuiteMeta               `yaml:",inline"`
	Rules     []ConditionFallbackRule `yaml:"rules,omitempty"`
	RegoRules []RegoRule              `yaml:"regos,omitempty"`
}

type yamlSuite struct {
	Meta  SuiteMeta                `yaml:",inline"`
	Rules []map[string]interface{} `yaml:"rules,omitempty"`
}

func (ys *yamlSuite) toSuite() (*Suite, error) {
	s := &Suite{}
	s.Meta = ys.Meta

	for _, rule := range ys.Rules {
		buffer, err := yaml.Marshal(rule)
		if err != nil {
			return nil, err
		}

		if _, ok := rule["input"]; ok {
			var regoRule RegoRule
			if err := yaml.Unmarshal(buffer, &regoRule); err != nil {
				return nil, err
			}
			s.RegoRules = append(s.RegoRules, regoRule)
		} else {
			var cfRule ConditionFallbackRule
			if err := yaml.Unmarshal(buffer, &cfRule); err != nil {
				return nil, err
			}
			s.Rules = append(s.Rules, cfRule)
		}
	}

	return s, nil
}

// ParseSuite loads a single compliance suite
func ParseSuite(config string) (*Suite, error) {
	c, err := semver.NewConstraint(versionConstraint)
	if err != nil {
		return nil, err
	}

	f, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}

	ys := &yamlSuite{}
	err = yaml.Unmarshal(f, ys)
	if err != nil {
		return nil, err
	}

	s, err := ys.toSuite()
	if err != nil {
		return nil, err
	}
	s.Meta.Source = config

	v, err := semver.NewVersion(s.Meta.Schema.Version)
	if err != nil {
		return nil, err
	}
	if !c.Check(v) {
		return nil, ErrUnsupportedSchemaVersion
	}
	return s, nil
}
