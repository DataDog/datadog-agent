// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package detector provides service metadata detection.
package detector

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// ServiceMetadata stores metadata about a service.
type ServiceMetadata struct {
	Name               string
	Language           string
	Type               string
	APMInstrumentation string
}

func fixAdditionalNames(additionalNames []string) []string {
	if len(additionalNames) == 0 {
		return additionalNames
	}

	out := make([]string, 0, len(additionalNames))
	for _, v := range additionalNames {
		if len(strings.TrimSpace(v)) == 0 {
			continue
		}

		// lang is only used for fallback names, which we don't use since we
		// check for errors.
		norm, err := normalize.NormalizeService(v, "")
		if err == nil {
			out = append(out, norm)
		}
	}
	slices.Sort(out)
	return out
}

// fixupMetadata performs additional adjustments on the meta data returned from
// the meta data extraction library.
func fixupMetadata(meta usm.ServiceMetadata, lang language.Language) usm.ServiceMetadata {
	meta.AdditionalNames = fixAdditionalNames(meta.AdditionalNames)

	langName := ""
	if lang != language.Unknown {
		langName = string(lang)
	}
	meta.Name, _ = normalize.NormalizeService(meta.Name, langName)
	if meta.DDService != "" {
		meta.DDService, _ = normalize.NormalizeService(meta.DDService, langName)
	}

	return meta
}

// GetServiceName gets the service name based on the command line arguments and
// the list of environment variables.
func GetServiceName(lang language.Language, ctx usm.DetectionContext) usm.ServiceMetadata {
	meta, _ := usm.ExtractServiceMetadata(lang, ctx)
	return fixupMetadata(meta, lang)
}
