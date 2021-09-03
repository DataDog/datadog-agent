// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package collectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"

	"github.com/containerd/containerd/oci"
	"github.com/gobwas/glob"
)

// extractTags generates container tags from its spec.
func (c *ContainderdCollector) extractTags(spec *oci.Spec) ([]string, []string, []string, []string) {
	if spec == nil || spec.Process == nil {
		return nil, nil, nil, nil
	}

	tags := utils.NewTagList()
	extractContainerdEnvVariables(tags, spec.Process.Env, c.envAsTags, c.globEnv)
	low, orchestrator, high, standard := tags.Compute()

	return low, orchestrator, high, standard
}

// extractContainerdEnvVariables uses the env as tags config to generate tagger tags.
func extractContainerdEnvVariables(tags *utils.TagList, envValues []string, envAsTags map[string]string, globEnv map[string]glob.Glob) {
	for _, env := range envValues {
		envSplit := strings.SplitN(env, "=", 2)
		if len(envSplit) != 2 {
			continue
		}

		// Env as tags
		utils.AddMetadataAsTags(envSplit[0], envSplit[1], envAsTags, globEnv, tags)
	}
}
