// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package aggregator

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func enrichTags(tb tagset.TagsAccumulator, udsOrigin string, clientOrigin string, cardinalityName string) {
	tagger.EnrichTags(tb, udsOrigin, clientOrigin, cardinalityName)
}

func agentTags(cardinality collectors.TagCardinality) ([]string, error) {
	return tagger.AgentTags(cardinality)
}

func globalTags(cardinality collectors.TagCardinality) ([]string, error) {
	return tagger.GlobalTags(cardinality)
}

func checkCardinality() collectors.TagCardinality {
	return tagger.ChecksCardinality
}
