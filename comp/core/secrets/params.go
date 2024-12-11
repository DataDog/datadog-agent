// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

package secrets

// Params contains parameters for secrets, specifically whether the component is enabled
type Params struct {
	Enabled bool
}

// NewEnabledParams constructs params for an enabled component
func NewEnabledParams() Params {
	return Params{
		Enabled: true,
	}
}

// NewDisabledParams constructs params for a disabled component
func NewDisabledParams() Params {
	return Params{
		Enabled: false,
	}
}
