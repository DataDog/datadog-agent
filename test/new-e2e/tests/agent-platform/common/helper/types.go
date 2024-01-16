// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helper implement interfaces to get some information that can be OS specific
package helper

// Helper generic interface
type Helper interface {
	GetInstallFolder() string
	GetConfigFolder() string
	GetBinaryPath() string
	GetConfigFileName() string
	GetServiceName() string
	AgentProcesses() []string
}
