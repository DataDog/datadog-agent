// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package wconfig holds wconfig related files
package wconfig

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// AllowPolicy defines an allow policy strategy
type AllowPolicy struct {
	Action string   `yaml:"action,omitempty"`
	Allow  []string `yaml:"allow,omitempty"`
}

// SECLPolicy defines a SECL rule based policy
type SECLPolicy struct {
	Rules []*rules.RuleDefinition `yaml:"rules,omitempty"`
}

// WorkloadPolicy defines a workload policy
type WorkloadPolicy struct {
	ID   string
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`

	AllowPolicy `yaml:",inline,omitempty"`
	SECLPolicy  `yaml:",inline,omitempty"`
}