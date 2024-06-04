// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	agentzap "github.com/DataDog/datadog-agent/pkg/util/log/zap"
)

type serviceDetector struct {
	logger     *zap.Logger
	langFinder language.Finder
}

func newServiceDetector() *serviceDetector {
	logger := zap.New(agentzap.NewZapCore())
	return &serviceDetector{
		logger:     logger,
		langFinder: language.New(logger),
	}
}

type serviceMetadata struct {
	Name               string
	Language           string
	Type               string
	APMInstrumentation string
	FromDDService      bool
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

func (sd *serviceDetector) Detect(p processInfo) serviceMetadata {
	meta, _ := usm.ExtractServiceMetadata(sd.logger, p.CmdLine, p.Env)
	lang, _ := sd.langFinder.Detect(p.CmdLine, p.Env)
	svcType := servicetype.Detect(meta.Name, p.Ports)
	apmInstr := apm.Detect(sd.logger, p.CmdLine, p.Env, lang)

	sd.logger.Debug("name info", zap.String("name", meta.Name), zap.Strings("additional names", meta.AdditionalNames))

	name := meta.Name
	if len(meta.AdditionalNames) > 0 {
		name = name + "-" + strings.Join(fixAdditionalNames(meta.AdditionalNames), "-")
	}

	return serviceMetadata{
		Name:               name,
		Language:           string(lang),
		Type:               string(svcType),
		APMInstrumentation: string(apmInstr),
		FromDDService:      meta.FromDDService,
	}
}
