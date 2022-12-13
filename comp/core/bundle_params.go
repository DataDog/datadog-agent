// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package core

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/internal"
)

// BundleParams defines the parameters for this bundle.
type BundleParams = internal.BundleParams

// CreateAgentBundleParams creates a new BundleParams for the Core Agent
func CreateAgentBundleParams(confFilePath string, configLoadSecrets bool, options ...func(*BundleParams)) BundleParams {
	params := CreateBundleParams(common.DefaultConfPath, options...)
	params.ConfFilePath = confFilePath
	params.ConfigLoadSecrets = configLoadSecrets
	return params
}

// CreateBundleParams creates a new BundleParams
func CreateBundleParams(defaultConfPath string, options ...func(*BundleParams)) BundleParams {
	bundleParams := BundleParams{
		DefaultConfPath: defaultConfPath,
	}
	for _, o := range options {
		o(&bundleParams)
	}
	return bundleParams
}

func WithConfigName(name string) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ConfigName = name
	}
}

func WithConfigMissingOK(v bool) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ConfigMissingOK = v
	}
}

func WithConfigLoadSysProbe(v bool) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ConfigLoadSysProbe = v
	}
}

func WithSecurityAgentConfigFilePaths(securityAgentConfigFilePaths []string) func(*BundleParams) {
	return func(b *BundleParams) {
		b.SecurityAgentConfigFilePaths = securityAgentConfigFilePaths
	}
}

func WithConfigLoadSecurityAgent(configLoadSecurityAgent bool) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ConfigLoadSecurityAgent = configLoadSecurityAgent
	}
}

func WithConfFilePath(confFilePath string) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ConfFilePath = confFilePath
	}
}

func WithConfigLoadSecrets(configLoadSecrets bool) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ConfigLoadSecrets = configLoadSecrets
	}
}

func WithLogForOneShot(loggerName, level string, overrideFromEnv bool) func(*BundleParams) {
	return func(b *BundleParams) {
		*b = b.LogForOneShot(loggerName, level, overrideFromEnv)
	}
}

func WithLogForDaemon(loggerName, logFileConfig, defaultLogFile string) func(*BundleParams) {
	return func(b *BundleParams) {
		*b = b.LogForDaemon(loggerName, logFileConfig, defaultLogFile)
	}
}

func WithLogToFile(logFile string) func(*BundleParams) {
	return func(b *BundleParams) {
		*b = b.LogToFile(logFile)
	}
}
