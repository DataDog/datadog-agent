// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !serverless

package otlp

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
)

func globalTags(cardinality collectors.TagCardinality) ([]string, error) {
	return tagger.GlobalTags(cardinality)
}

func tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	return tagger.Tag(entity, cardinality)
}
