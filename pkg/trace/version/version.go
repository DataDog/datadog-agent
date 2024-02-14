package version

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"strings"
)

/*
This package comprises functions that are used to retrieve *app* version data from incoming traces.
*/

const (
	versionField          = "version"
	gitCommitShaField     = "_dd.git.commit.sha"
	gitCommitShaTagPrefix = "git.commit.sha:"
	imageTagPrefix        = "image_tag:"
)

// GetVersionDataFromContainerTags will return the git commit sha and image tag from container tags, if present.
func GetVersionDataFromContainerTags(containerID string, conf *config.AgentConfig) (gitCommitSha, imageTag string, err error) {
	cTags, err := conf.ContainerTags(containerID)
	if err != nil {
		return "", "", err
	}
	for _, t := range cTags {
		if gitCommitSha != "" && imageTag != "" {
			break
		}
		if val, ok := strings.CutPrefix(t, gitCommitShaTagPrefix); ok && val != "" {
			gitCommitSha = val
			continue
		}
		if val, ok := strings.CutPrefix(t, imageTagPrefix); ok && val != "" {
			imageTag = val
		}
	}
	return gitCommitSha, imageTag, nil
}

// GetGitCommitShaFromTrace returns the first "git_commit_sha" tag found in trace t.
func GetGitCommitShaFromTrace(root *trace.Span, t *trace.TraceChunk) string {
	if v, ok := root.Meta[gitCommitShaField]; ok {
		return v
	}
	for _, s := range t.Spans {
		if s.SpanID == root.SpanID {
			continue
		}
		if v, ok := s.Meta[gitCommitShaField]; ok {
			return v
		}
	}
	return ""
}

// GetAppVersionFromTrace returns the first "version" tag found in trace t.
// Search starts by root
func GetAppVersionFromTrace(root *trace.Span, t *trace.TraceChunk) string {
	if v, ok := root.Meta[versionField]; ok {
		return v
	}
	for _, s := range t.Spans {
		if s.SpanID == root.SpanID {
			continue
		}
		if v, ok := s.Meta[versionField]; ok {
			return v
		}
	}
	return ""
}
