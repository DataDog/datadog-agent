// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package env

import "github.com/DataDog/datadog-agent/pkg/compliance"

// Env provides environment methods for compliance checks execution
type Env interface {
	Clients
	Configuration
	Reporter() compliance.Reporter
}

// Clients provides an abstraction for accessing various clients needed by checks
type Clients interface {
	DockerClient() DockerClient
	AuditClient() AuditClient
	KubeClient() KubeClient
}

// Configuration provides an abstraction for various environment methods used by checks
type Configuration interface {
	Hostname() string
	EtcGroupPath() string
	NormalizePath(path string) string
	ResolveValueFrom(valueFrom compliance.ValueFrom) (string, error)
}
