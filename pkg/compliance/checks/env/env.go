// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
)

// Env provides environment methods for compliance checks execution
type Env interface {
	Clients
	Configuration
	RegoConfiguration
	Reporter() event.Reporter
}

// Clients provides an abstraction for accessing various clients needed by checks
type Clients interface {
	DockerClient() DockerClient
	AuditClient() AuditClient
	KubeClient() KubeClient
}

// RegoConfiguration provides the rego specific configuration
type RegoConfiguration interface {
	ProvidedInput(ruleID string) eval.RegoInputMap
	DumpInputPath() string
}

// Configuration provides an abstraction for various environment methods used by checks
type Configuration interface {
	Hostname() string
	MaxEventsPerRun() int
	EtcGroupPath() string
	NormalizeToHostRoot(path string) string
	RelativeToHostRoot(path string) string
	EvaluateFromCache(e eval.Evaluatable) (interface{}, error)
	IsLeader() bool
	NodeLabels() map[string]string
}
