// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobeconfigimpl

// Params defines the parameters for the sysprobeconfig component.
type Params struct {
	// sysProbeConfFilePath is the path at which to look for configuration, usually
	// given by the --sysprobecfgpath command-line flag.
	sysProbeConfFilePath string
}

// NewParams creates a new instance of Params
func NewParams(options ...func(*Params)) Params {
	params := Params{}
	for _, o := range options {
		o(&params)
	}
	return params
}

// WithSysProbeConfFilePath specifies the path to the system probe config
func WithSysProbeConfFilePath(confFilePath string) func(*Params) {
	return func(b *Params) {
		b.sysProbeConfFilePath = confFilePath
	}
}
