// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
)

// ServiceMetadata stores metadata about a service.
type ServiceMetadata struct {
	Name               string
	Language           string
	Type               string
	APMInstrumentation string
	NameSource         string
}

func fixAdditionalNames(additionalNames []string) []string {
	out := make([]string, 0, len(additionalNames))
	for _, v := range additionalNames {
		if len(strings.TrimSpace(v)) > 0 {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

func makeFinalName(meta usm.ServiceMetadata) string {
	name := meta.Name
	if len(meta.AdditionalNames) > 0 {
		name = name + "-" + strings.Join(fixAdditionalNames(meta.AdditionalNames), "-")
	}

	return name
}

// GetServiceName gets the service name based on the command line arguments and
// the list of environment variables.
func GetServiceName(cmdline []string, env map[string]string) string {
	meta, _ := usm.ExtractServiceMetadata(cmdline, env)
	return makeFinalName(meta)
}
