// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

var validProducts = map[string]struct{}{
	ProductAPMSampling: {},
	ProductCWSDD:       {},
	ProductCWSCustom:   {},
	ProductCWSProfiles: {},
	ProductASMFeatures: {},
	ProductASMDD:       {},
	ProductASMData:     {},
	ProductAPMTracing:  {},
}

const (
	// ProductAPMSampling is the apm sampling product
	ProductAPMSampling = "APM_SAMPLING"
	// ProductCWSDD is the cloud workload security product managed by datadog employees
	ProductCWSDD = "CWS_DD"
	// ProductCWSCustom is the cloud workload security product managed by datadog customers
	ProductCWSCustom = "CWS_CUSTOM"
	// ProductCWSProfile is the cloud workload security profile product
	ProductCWSProfiles = "CWS_SECURITY_PROFILES"
	// ProductASMFeatures is the ASM product used form ASM activation through remote config
	ProductASMFeatures = "ASM_FEATURES"
	// ProductASMDD is the application security monitoring product managed by datadog employees
	ProductASMDD = "ASM_DD"
	// ProductASMData is the ASM product used to configure WAF rules data
	ProductASMData = "ASM_DATA"
	// ProductAPMTracing is the apm tracing product
	ProductAPMTracing = "APM_TRACING"
)
