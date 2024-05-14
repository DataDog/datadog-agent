// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
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
	Name     string
	Language string
	Type     string
}

func (sd *serviceDetector) Detect(p processInfo) serviceMetadata {
	meta, _ := usm.ExtractServiceMetadata(usm.NewDetectionContext(sd.logger, p.CmdLine, p.Env, usm.RealFs{}))
	lang, _ := sd.langFinder.Detect(p.CmdLine, p.Env)
	svcType := servicetype.Detect(meta.Name, p.Ports)

	return serviceMetadata{
		Name:     meta.Name,
		Language: string(lang),
		Type:     string(svcType),
	}
}
