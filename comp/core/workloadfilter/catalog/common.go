// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

// This file contains filter programs that can be shared across different entity types.

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// AutodiscoveryAnnotations creates a CEL program for autodiscovery annotations.
func AutodiscoveryAnnotations() program.AnnotationsProgram {
	return program.AnnotationsProgram{
		Name:          "AutodiscoveryAnnotation",
		ExcludePrefix: "",
	}
}

// AutodiscoveryMetricsAnnotations creates a CEL program for autodiscovery metrics annotations.
func AutodiscoveryMetricsAnnotations() program.AnnotationsProgram {
	return program.AnnotationsProgram{
		Name:          "AutodiscoveryMetricsAnnotations",
		ExcludePrefix: "metrics_",
	}
}

// AutodiscoveryLogsAnnotations creates a CEL program for autodiscovery logs annotations.
func AutodiscoveryLogsAnnotations() program.AnnotationsProgram {
	return program.AnnotationsProgram{
		Name:          "AutodiscoveryLogsAnnotations",
		ExcludePrefix: "logs_",
	}
}
