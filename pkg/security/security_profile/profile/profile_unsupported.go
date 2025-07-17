// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package profile holds profile related files
package profile

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

// Profile is defined for unsupported platforms
type Profile struct{}

// GetImageNameTag returns the image name and tag of this activity dump
func (p *Profile) GetImageNameTag() (string, string) {
	return "", ""
}

// NewProfileFromActivityDumpMessage returns a new Profile from a ActivityDumpMessage.
func NewProfileFromActivityDumpMessage(_ *api.ActivityDumpMessage) (*Profile, map[config.StorageFormat][]config.StorageRequest, error) {
	return nil, nil, errors.New("activity dumps are unsupported on this platform")
}
