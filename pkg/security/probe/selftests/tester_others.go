// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package selftests

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(cfg *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {
	return &SelfTester{
		probe:  probe,
		config: cfg,
	}, nil
}
