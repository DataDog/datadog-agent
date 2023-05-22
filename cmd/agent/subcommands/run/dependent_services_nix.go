// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package run

import "github.com/DataDog/datadog-agent/pkg/config"

// Servicedef defines a service
type Servicedef struct {
	name       string
	configKeys map[string]config.Config
}

var subservices []Servicedef

// Start starts the service
func (s *Servicedef) Start() error {
	return nil
}

// Stop stops the service
func (s *Servicedef) Stop() error {
	return nil
}
