// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package hostinfo

import (
	"context"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// TagRuleNodeAnnotationPrefix is the reserved annotation prefix the Cluster
// Agent tag-rule controller uses to write rule-owned tags onto nodes: every
// node annotation "tags.datadoghq.com/tag.<tag-key>: <value>" maps to the
// host tag "<tag-key>:<value>", with no agent configuration required.
const TagRuleNodeAnnotationPrefix = "tags.datadoghq.com/tag."

// GetTagRuleNodeTags returns the tag-rule tags currently written on this
// host's node.
//
// The annotations are fetched from the API server with a single-object GET
// (one small request per agent per refresh cadence): the Cluster Agent
// annotations endpoint cannot serve them because it applies an exact-match
// filter (defaulting to host aliases) server-side. Callers are expected to
// invoke this on a short cadence and to degrade gracefully on error.
func GetTagRuleNodeTags(ctx context.Context) ([]string, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	nodeName, err := ku.GetNodename(ctx)
	if err != nil {
		return nil, err
	}

	nodeAnnotations, err := apiserverNodeAnnotations(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	return extractTagRuleTags(nodeAnnotations), nil
}

// extractTagRuleTags maps reserved-prefix annotations to tag-rule tags,
// sorted for determinism. It returns nil when no annotation matches.
func extractTagRuleTags(annotations map[string]string) []string {
	var tags []string
	for name, value := range annotations {
		if key, found := strings.CutPrefix(name, TagRuleNodeAnnotationPrefix); found && key != "" {
			tags = append(tags, key+":"+value)
		}
	}
	slices.Sort(tags)
	return tags
}
