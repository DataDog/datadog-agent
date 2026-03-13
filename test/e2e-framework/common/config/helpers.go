// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func FindEnvironmentName(environments []string, prefix string) string {
	for _, env := range environments {
		if strings.HasPrefix(env, prefix+"/") {
			return env
		}
	}
	return ""
}

func anyAgentSemverVersion(version string) (*semver.Version, error) {
	if version == "" {
		return nil, nil
	}

	return semver.NewVersion(version)
}

func AgentSemverVersion(e Env) (*semver.Version, error) {
	return anyAgentSemverVersion(e.AgentVersion())
}

func ClusterAgentSemverVersion(e Env) (*semver.Version, error) {
	return anyAgentSemverVersion(e.ClusterAgentVersion())
}

func OperatorSemverVersion(e Env) (*semver.Version, error) {
	return anyAgentSemverVersion(e.OperatorVersion())
}

func tagListToKeyValueMap(tagList []string) (map[string]string, error) {
	tags := map[string]string{}
	for _, tag := range tagList {
		keyAndValue := strings.Split(tag, ":")
		if len(keyAndValue) != 2 {
			// skip invalid tags
			return tags, fmt.Errorf("invalid tag, expecting <key>:<value>, got %s", tag)
		}
		tags[keyAndValue[0]] = keyAndValue[1]
	}
	return tags, nil
}

func extendTagsMap(pulumiStringMap pulumi.StringMap, otherMap map[string]string) {
	for key, value := range otherMap {
		pulumiStringMap[strings.ReplaceAll(
			strings.ToLower(key), "_", "-")] = pulumi.String(value)
	}
}
