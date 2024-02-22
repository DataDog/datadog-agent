// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package aggregator

import "github.com/DataDog/datadog-agent/pkg/tagset"
import "github.com/DataDog/datadog-agent/comp/core/tagger/collectors"

func enrichTags(tb tagset.TagsAccumulator, udsOrigin string, clientOrigin string, cardinalityName string) {
	// nothing to do here
}

func agentTags(cardinality collectors.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func globalTags(cardinality collectors.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func checkCardinality() collectors.TagCardinality {
	return 0
}
