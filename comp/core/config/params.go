// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "github.com/DataDog/datadog-agent/cmd/agent/common"

// Params defines the parameters for the config component.
type Params struct {
	// confFilePath is the path at which to look for configuration, usually
	// given by the --cfgpath command-line flag.
	confFilePath string

	// configName is the root of the name of the configuration file.  The
	// comp/core/config component will search for a file with this name
	// in ConfFilePath, using a variety of extensions.  The default is
	// "datadog".
	configName string

	// securityAgentConfigFilePaths are the paths at which to look for security-aegnt
	// configuration, usually given by the --cfgpath command-line flag.
	securityAgentConfigFilePaths []string

	// configLoadSecurityAgent determines whether to read the config from
	// SecurityAgentConfigFilePaths or from ConfFilePath.
	configLoadSecurityAgent bool

	// ConfigLoadSecrets determines whether secrets in the configuration file
	// should be evaluated.  This is typically false for one-shot commands.
	configLoadSecrets bool

	// configMissingOK determines whether it is a fatal error if the config
	// file does not exist.
	configMissingOK bool

	// defaultConfPath determines the default configuration path.
	// if defaultConfPath is empty, then no default configuration path is used.
	defaultConfPath string
}

// NewParams creates a new instance of Params
func NewParams(defaultConfPath string, options ...func(*Params)) Params {
	params := Params{
		defaultConfPath: defaultConfPath,
	}
	for _, o := range options {
		o(&params)
	}
	return params
}

// NewAgentParamsWithSecrets creates a new instance of Params using secrets for the Agent.
func NewAgentParamsWithSecrets(confFilePath string, options ...func(*Params)) Params {
	return newAgentParams(confFilePath, true, options...)
}

// NewAgentParamsWithoutSecrets creates a new instance of Params without using secrets for the Agent.
func NewAgentParamsWithoutSecrets(confFilePath string, options ...func(*Params)) Params {
	return newAgentParams(confFilePath, false, options...)
}

func newAgentParams(confFilePath string, configLoadSecrets bool, options ...func(*Params)) Params {
	params := NewParams(common.DefaultConfPath, options...)
	params.confFilePath = confFilePath
	params.configLoadSecrets = configLoadSecrets
	return params
}

// NewSecurityAgentParams creates a new instance of Params for the Security Agent.
func NewSecurityAgentParams(securityAgentConfigFilePaths []string, options ...func(*Params)) Params {
	params := NewParams(common.DefaultConfPath, options...)

	// By default, we load datadog.yaml and then merge security-agent.yaml
	if len(securityAgentConfigFilePaths) > 0 {
		params.confFilePath = securityAgentConfigFilePaths[0]                  // Default: datadog.yaml
		params.securityAgentConfigFilePaths = securityAgentConfigFilePaths[1:] // Default: security-agent.yaml
	}
	params.configLoadSecurityAgent = true

	params.configLoadSecrets = true
	params.configMissingOK = false
	return params
}

func NewClusterAgentParams(configFilePath string, options ...func(*Params)) Params {
	return NewParams(common.DefaultConfPath,
		WithConfFilePath(configFilePath),
		WithConfigName("datadog-cluster"))
}

func WithConfigName(name string) func(*Params) {
	return func(b *Params) {
		b.configName = name
	}
}

func WithConfigMissingOK(v bool) func(*Params) {
	return func(b *Params) {
		b.configMissingOK = v
	}
}

func WithSecurityAgentConfigFilePaths(securityAgentConfigFilePaths []string) func(*Params) {
	return func(b *Params) {
		b.securityAgentConfigFilePaths = securityAgentConfigFilePaths
	}
}

func WithConfigLoadSecurityAgent(configLoadSecurityAgent bool) func(*Params) {
	return func(b *Params) {
		b.configLoadSecurityAgent = configLoadSecurityAgent
	}
}

func WithConfFilePath(confFilePath string) func(*Params) {
	return func(b *Params) {
		b.confFilePath = confFilePath
	}
}

func WithConfigLoadSecrets(configLoadSecrets bool) func(*Params) {
	return func(b *Params) {
		b.configLoadSecrets = configLoadSecrets
	}
}

// These functions are used in unit tests.

// ConfigLoadSecrets determines whether secrets in the configuration file
// should be evaluated.  This is typically false for one-shot commands.
func (p Params) ConfigLoadSecrets() bool {
	return p.configLoadSecrets
}

// ConfigMissingOK determines whether it is a fatal error if the config
// file does not exist.
func (p Params) ConfigMissingOK() bool {
	return p.configMissingOK
}
