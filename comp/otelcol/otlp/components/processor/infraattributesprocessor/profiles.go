// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package infraattributesprocessor implements the same logic for profiles as metrics and traces.
package infraattributesprocessor

import (
	"context"
	"runtime"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesProfileProcessor struct {
	infraTags   infraTagsProcessor
	logger      *zap.Logger
	cardinality types.TagCardinality
	cfg         *Config
}

func newInfraAttributesProfileProcessor(
	set processor.Settings,
	infraTags infraTagsProcessor,
	cfg *Config,
) (*infraAttributesProfileProcessor, error) {
	iapp := &infraAttributesProfileProcessor{
		infraTags:   infraTags,
		logger:      set.Logger,
		cardinality: cfg.Cardinality,
		cfg:         cfg,
	}
	set.Logger.Info("Profile Infra Attributes Processor configured")
	return iapp, nil
}

func (iapp *infraAttributesProfileProcessor) processProfiles(_ context.Context, pd pprofile.Profiles) (pprofile.Profiles, error) {
	rps := pd.ResourceProfiles()
	for i := 0; i < rps.Len(); i++ {
		resourceAttributes := rps.At(i).Resource().Attributes()

		// Always add `host.arch` tag to profiles' resource attributes
		resourceAttributes.PutStr("host.arch", runtime.GOARCH)

		iapp.infraTags.ProcessTags(iapp.logger, iapp.cardinality, resourceAttributes, iapp.cfg.AllowHostnameOverride)
	}
	return pd, nil
}
