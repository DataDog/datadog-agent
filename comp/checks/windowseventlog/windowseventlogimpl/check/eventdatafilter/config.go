// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package eventdatafilter

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

const (
	schemaMajorVersion = 1
)

type versionSchema struct {
	SchemaVersion string `yaml:"schema_version"`
}

type eventDataFilterSchema struct {
	EventIDs []int `yaml:"eventid"`
}

func unmarshalEventdataFilterSchema(config []byte) (*eventDataFilterSchema, error) {
	// Check schema version
	var version versionSchema
	err := yaml.Unmarshal(config, &version)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal schema_version: %w", err)
	}
	if version.SchemaVersion == "" {
		return nil, fmt.Errorf("schema_version is required but is missing or empty")
	}
	supported, err := supportedVersion(version.SchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("could not parse schema_version: %w", err)
	}
	if !supported {
		return nil, fmt.Errorf("unsupported schema version %s, please use a version compatible with version %d", version.SchemaVersion, schemaMajorVersion)
	}

	var filter eventDataFilterSchema
	err = yaml.Unmarshal(config, &filter)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal eventdata filter: %w", err)
	}
	return &filter, nil
}

func supportedVersion(version string) (bool, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, err
	}
	return v.Major() == schemaMajorVersion, nil
}
