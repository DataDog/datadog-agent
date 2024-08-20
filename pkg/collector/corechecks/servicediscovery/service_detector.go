// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServiceDetector defines the service detector to get metadata about services.
type ServiceDetector struct {
	langFinder language.Finder
}

// NewServiceDetector creates a new ServiceDetector object.
func NewServiceDetector() *ServiceDetector {
	return &ServiceDetector{
		langFinder: language.New(),
	}
}

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
func (sd *ServiceDetector) GetServiceName(cmdline []string, env map[string]string) string {
	meta, _ := usm.ExtractServiceMetadata(cmdline, env)
	return makeFinalName(meta)
}

// Detect gets metadata for a service.
func (sd *ServiceDetector) Detect(p processInfo) ServiceMetadata {
	meta, _ := usm.ExtractServiceMetadata(p.CmdLine, p.Env)
	lang, _ := sd.langFinder.Detect(p.CmdLine, p.Env)
	svcType := servicetype.Detect(meta.Name, p.Ports)
	apmInstr := apm.Detect(p.PID, p.CmdLine, p.Env, lang)

	log.Debugf("name info - name: %q; additional names: %v", meta.Name, meta.AdditionalNames)

	nameSource := "generated"
	if meta.FromDDService {
		nameSource = "provided"
	}

	return ServiceMetadata{
		Name:               makeFinalName(meta),
		Language:           string(lang),
		Type:               string(svcType),
		APMInstrumentation: string(apmInstr),
		NameSource:         nameSource,
	}
}
