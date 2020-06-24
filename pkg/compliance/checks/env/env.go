// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package env

import "github.com/DataDog/datadog-agent/pkg/compliance"

// Env provides an abstraction for various environment methods needed by checks
type Env interface {
	Reporter() compliance.Reporter
	DockerClient() DockerClient
	AuditClient() AuditClient
	KubeClient() KubeClient

	Hostname() string
	EtcGroupPath() string
	NormalizePath(path string) string
	ResolveValueFrom(valueFrom compliance.ValueFrom) (string, error)
}
