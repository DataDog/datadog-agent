// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// Params defines the parameters for the config component.
type Params struct {
	// ConfFilePath is the path at which to look for configuration, usually
	// given by the --cfgpath command-line flag.
	ConfFilePath string

	// ExtraConfFilePath represents the paths to additional configuration files to be merged over the main datadog.yaml.
	// Usually given by the --extracfgpath command-line flag.
	ExtraConfFilePath []string

	// FleetPoliciesDirPath is the path at which to look for remote configuration files
	FleetPoliciesDirPath string

	// ConfigName is the root of the name of the configuration file.  The
	// comp/core/config component will search for a file with this name
	// in ConfFilePath, using a variety of extensions.  The default is
	// "datadog".
	ConfigName string

	// SecurityAgentConfigFilePaths are the paths at which to look for security-agent
	// configuration, usually given by the --cfgpath command-line flag.
	SecurityAgentConfigFilePaths []string

	// ConfigLoadSecurityAgent determines whether to read the config from
	// SecurityAgentConfigFilePaths or from ConfFilePath.
	ConfigLoadSecurityAgent bool

	// ConfigMissingOK determines whether it is a fatal error if the config
	// file does not exist.
	ConfigMissingOK bool

	// IgnoreErrors determines whether it is OK if the config is not valid
	// If an error occurs, Component.warnings.Warning contains the error.
	IgnoreErrors bool

	// DefaultConfPath determines the default configuration path.
	// if DefaultConfPath is empty, then no default configuration path is used.
	DefaultConfPath string

	// CLIOverride is a list of setting overrides from the CLI given to the configuration. The map associate
	// settings name like "logs_config.enabled" to its value.
	CLIOverride map[string]interface{}
}

// NewParams creates a new instance of Params
func NewParams(defaultConfPath string, options ...func(*Params)) Params {
	params := Params{
		DefaultConfPath: defaultConfPath,
		CLIOverride:     map[string]interface{}{},
	}
	for _, o := range options {
		o(&params)
	}
	return params
}

// NewAgentParams creates a new instance of Params for the Agent.
func NewAgentParams(confFilePath string, options ...func(*Params)) Params {
	params := NewParams(DefaultConfPath, options...)
	params.ConfFilePath = confFilePath
	return params
}

// NewSecurityAgentParams creates a new instance of Params for the Security Agent.
func NewSecurityAgentParams(securityAgentConfigFilePaths []string, options ...func(*Params)) Params {
	params := NewParams(DefaultConfPath, options...)

	// By default, we load datadog.yaml and then merge security-agent.yaml
	if len(securityAgentConfigFilePaths) > 0 {
		params.ConfFilePath = securityAgentConfigFilePaths[0]                  // Default: datadog.yaml
		params.SecurityAgentConfigFilePaths = securityAgentConfigFilePaths[1:] // Default: security-agent.yaml
	}
	params.ConfigLoadSecurityAgent = true
	params.ConfigMissingOK = false
	return params
}

// NewClusterAgentParams returns a new Params struct for the cluster agent
func NewClusterAgentParams(configFilePath string, options ...func(*Params)) Params {
	params := NewParams(DefaultConfPath, options...)
	params.ConfFilePath = configFilePath
	params.ConfigName = "datadog-cluster"
	return params
}

// WithConfigName returns an option which sets the config name
func WithConfigName(name string) func(*Params) {
	return func(b *Params) {
		b.ConfigName = name
	}
}

// WithConfigMissingOK returns an option which sets ConfigMissingOK
func WithConfigMissingOK(v bool) func(*Params) {
	return func(b *Params) {
		b.ConfigMissingOK = v
	}
}

// WithIgnoreErrors returns an option which sets IgnoreErrors
func WithIgnoreErrors(v bool) func(*Params) {
	return func(b *Params) {
		b.IgnoreErrors = v
	}
}

// WithSecurityAgentConfigFilePaths returns an option which sets SecurityAgentConfigFilePaths
func WithSecurityAgentConfigFilePaths(securityAgentConfigFilePaths []string) func(*Params) {
	return func(b *Params) {
		b.SecurityAgentConfigFilePaths = securityAgentConfigFilePaths
	}
}

// WithConfigLoadSecurityAgent returns an option which sets ConfigLoadSecurityAgent
func WithConfigLoadSecurityAgent(configLoadSecurityAgent bool) func(*Params) {
	return func(b *Params) {
		b.ConfigLoadSecurityAgent = configLoadSecurityAgent
	}
}

// WithConfFilePath returns an option which sets ConfFilePath
func WithConfFilePath(confFilePath string) func(*Params) {
	return func(b *Params) {
		b.ConfFilePath = confFilePath
	}
}

// WithExtraConfFiles returns an option which sets ConfFilePath
func WithExtraConfFiles(extraConfFilePath []string) func(*Params) {
	return func(b *Params) {
		b.ExtraConfFilePath = extraConfFilePath
	}
}

// WithFleetPoliciesDirPath returns an option which sets FleetPoliciesDirPath
func WithFleetPoliciesDirPath(fleetPoliciesDirPath string) func(*Params) {
	return func(b *Params) {
		b.FleetPoliciesDirPath = fleetPoliciesDirPath
	}
}

// WithCLIOverride registers a list of settings overrides from the CLI for the configuration. The map associate settings
// name like "logs_config.enabled" to its value.
func WithCLIOverride(setting string, value interface{}) func(*Params) {
	return func(b *Params) {
		b.CLIOverride[setting] = value
	}
}

// These functions are used in unit tests.

// GetConfigMissingOK determines whether it is a fatal error if the config
// file does not exist.
func (p Params) GetConfigMissingOK() bool {
	return p.ConfigMissingOK
}
