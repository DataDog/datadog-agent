// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

var (
	// cumulativetodelta
	cumulativeToDeltaName         = "cumulativetodelta"
	cumulativeToDeltaEnhancedName = cumulativeToDeltaName + "/" + ddAutoconfiguredSuffix
	// cumulativeToDeltaConfig is left nil so the processor uses its default config,
	// which converts ALL cumulative metric types (sum + histogram + exponentialhistogram)
	// with MaxStaleness=1h. See OTAGENT-1128.
	cumulativeToDeltaConfig any

	// component
	cumulativeToDeltaProcessor = component{
		Name:         cumulativeToDeltaName,
		EnhancedName: cumulativeToDeltaEnhancedName,
		Type:         "processors",
		Config:       cumulativeToDeltaConfig,
	}
)
