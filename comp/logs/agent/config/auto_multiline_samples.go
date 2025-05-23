// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// AutoMultilineSample defines a sample used to create auto multiline detection
// rules
type AutoMultilineSample struct {
	Sample         string
	Label          *string
	Regex          string
	MatchThreshold *float64 `mapstructure:"match_threshold,omitempty" json:"match_threshold,omitempty" yaml:"match_threshold,omitempty"`
}
