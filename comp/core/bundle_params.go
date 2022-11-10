// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package core

import (
	"github.com/DataDog/datadog-agent/comp/core/internal"
)

// BundleParams defines the parameters for this bundle.
type BundleParams = internal.BundleParams

func CreateAgentBundleParams(confFilePath string, configLoadSecrets bool, options ...func(*BundleParams)) BundleParams {
	params := CreateBundleParams(options...)
	params.ConfFilePath = confFilePath
	params.ConfigLoadSecrets = configLoadSecrets
	return params
}

func CreateBundleParams(options ...func(*BundleParams)) BundleParams {
	bundleParams := BundleParams{}
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

func WithExcludeDefaultConfPath(excludeDefaultConfPath bool) func(*BundleParams) {
	return func(b *BundleParams) {
		b.ExcludeDefaultConfPath = excludeDefaultConfPath
	}
}

func WithDefaultConfPath(defaultConfPath string) func(*BundleParams) {
	return func(b *BundleParams) {
		b.DefaultConfPath = defaultConfPath
	}
}
