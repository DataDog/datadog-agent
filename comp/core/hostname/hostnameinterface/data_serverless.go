// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package hostnameinterface

// IsConfigurationProvider returns false for serverless
func (h Data) FromConfiguration() bool {
	return false
}

// fromFargate returns true if the hostname was found through Fargate
func (h Data) FromFargate() bool {
	return false
}
