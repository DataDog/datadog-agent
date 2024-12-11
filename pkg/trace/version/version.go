// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package version comprises functions that are used to retrieve *app* version data from incoming traces.
package version

import (
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

const (
	versionField          = "version"
	gitCommitShaField     = "_dd.git.commit.sha"
	gitCommitShaTagPrefix = "git.commit.sha:"
	imageTagPrefix        = "image_tag:"
)

// GetVersionDataFromContainerTags will return the git commit sha and image tag from container tags, if present.
func GetVersionDataFromContainerTags(containerID string, conf *config.AgentConfig) (gitCommitSha, imageTag string, err error) {
	if conf == nil || conf.ContainerTags == nil {
		return "", "", nil
	}
	cTags, err := conf.ContainerTags(containerID)
	if err != nil {
		if errors.Is(err, config.ErrContainerTagsFuncNotDefined) {
			return "", "", nil
		}
		return "", "", err
	}
	for _, t := range cTags {
		if gitCommitSha == "" {
			if sha, ok := strings.CutPrefix(t, gitCommitShaTagPrefix); ok {
				gitCommitSha = sha
			}
		}
		if imageTag == "" {
			if image, ok := strings.CutPrefix(t, imageTagPrefix); ok {
				imageTag = image
			}
		}
		if gitCommitSha != "" && imageTag != "" {
			break
		}
	}
	return gitCommitSha, imageTag, nil
}

// GetGitCommitShaFromTrace returns the first "git_commit_sha" tag found in trace t.
func GetGitCommitShaFromTrace(root *trace.Span, t *trace.TraceChunk) string {
	return searchTraceForField(root, t, gitCommitShaField)
}

// GetAppVersionFromTrace returns the first "version" tag found in trace t.
// Search starts by root
func GetAppVersionFromTrace(root *trace.Span, t *trace.TraceChunk) string {
	return searchTraceForField(root, t, versionField)
}

func searchTraceForField(root *trace.Span, t *trace.TraceChunk, field string) string {
	if v, ok := root.Meta[field]; ok {
		return v
	}
	for _, s := range t.Spans {
		if s.SpanID == root.SpanID {
			continue
		}
		if v, ok := s.Meta[field]; ok {
			return v
		}
	}
	return ""
}
