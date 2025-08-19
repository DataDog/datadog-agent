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

	// ignoreErrors determines whether it is OK if the config is not valid
	// If an error occurs, Component.warnings.Warning contains the error.
	ignoreErrors bool

	// defaultConfPath determines the default configuration path.
	// if defaultConfPath is empty, then no default configuration path is used.
	defaultConfPath string

	// cliOverride is a list of setting overrides from the CLI given to the configuration. The map associate
	// settings name like "logs_config.enabled" to its value.
	cliOverride map[string]interface{}
}

// NewParams creates a new instance of Params
func NewParams(defaultConfPath string, options ...func(*Params)) Params {
	params := Params{
		defaultConfPath: defaultConfPath,
		cliOverride:     map[string]interface{}{},
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
		params.securityAgentConfigFilePaths = securityAgentConfigFilePaths[1:] // Default: security-agent.yaml
	}
	params.configLoadSecurityAgent = true
	return params
}

// NewClusterAgentParams returns a new Params struct for the cluster agent
func NewClusterAgentParams(configFilePath string, options ...func(*Params)) Params {
	params := NewParams(DefaultConfPath, options...)
	params.ConfFilePath = configFilePath
	params.configName = "datadog-cluster"
	return params
}

// WithConfigName returns an option which sets the config name
func WithConfigName(name string) func(*Params) {
	return func(b *Params) {
		b.configName = name
	}
}

// WithIgnoreErrors returns an option which sets ignoreErrors
func WithIgnoreErrors(v bool) func(*Params) {
	return func(b *Params) {
		b.ignoreErrors = v
	}
}

// WithSecurityAgentConfigFilePaths returns an option which sets securityAgentConfigFilePaths
func WithSecurityAgentConfigFilePaths(securityAgentConfigFilePaths []string) func(*Params) {
	return func(b *Params) {
		b.securityAgentConfigFilePaths = securityAgentConfigFilePaths
	}
}

// WithConfigLoadSecurityAgent returns an option which sets configLoadSecurityAgent
func WithConfigLoadSecurityAgent(configLoadSecurityAgent bool) func(*Params) {
	return func(b *Params) {
		b.configLoadSecurityAgent = configLoadSecurityAgent
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
		b.cliOverride[setting] = value
	}
}
