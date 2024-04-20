// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"fmt"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func TestGetVersionDataFromContainerTags(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		cfg := config.New()
		cfg.ContainerTags = func(cid string) ([]string, error) {
			if cid == "1" {
				return []string{"some_tag:blah", "image_tag:x", "git.commit.sha:y"}, nil
			}
			return nil, nil
		}
		gitCommitSha, imageTag, err := GetVersionDataFromContainerTags("1", cfg)
		assert.NoError(t, err)
		assert.Equal(t, "x", imageTag)
		assert.Equal(t, "y", gitCommitSha)
		gitCommitSha, imageTag, err = GetVersionDataFromContainerTags("2", cfg)
		assert.NoError(t, err)
		assert.Equal(t, "", imageTag)
		assert.Equal(t, "", gitCommitSha)
	})
	t.Run("error", func(t *testing.T) {
		cfg := config.New()
		cfg.ContainerTags = func(cid string) ([]string, error) {
			return nil, fmt.Errorf("error")
		}
		gitCommitSha, imageTag, err := GetVersionDataFromContainerTags("1", cfg)
		assert.Error(t, err)
		assert.Equal(t, "", imageTag)
		assert.Equal(t, "", gitCommitSha)
	})
	t.Run("undefined", func(t *testing.T) {
		cfg := config.New()
		gitCommitSha, imageTag, err := GetVersionDataFromContainerTags("1", cfg)
		assert.NoError(t, err)
		assert.Equal(t, "", imageTag)
		assert.Equal(t, "", gitCommitSha)
	})
}

func TestGetGitCommitShaFromTrace(t *testing.T) {
	tts := []struct {
		name     string
		in       pb.Trace
		expected string
	}{
		{
			name: "no-git-commit-sha",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0},
			},
			expected: "",
		},
		{
			name: "root_git_commit_sha",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0, Meta: map[string]string{"_dd.git.commit.sha": "abc123456789"}},
			},
			expected: "abc123456789",
		},
		{
			name: "version",
			in: pb.Trace{
				&pb.Span{SpanID: 24, ParentID: 5, Meta: map[string]string{"_dd.git.commit.sha": "abc123456789"}},
				&pb.Span{ParentID: 0},
			},
			expected: "abc123456789",
		},
	}

	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, GetGitCommitShaFromTrace(traceutil.GetRoot(tc.in), &pb.TraceChunk{Spans: tc.in}))
		})
	}
}

func TestGetAppVersionFromTrace(t *testing.T) {
	tts := []struct {
		name     string
		in       pb.Trace
		expected string
	}{
		{
			name: "no-version",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0},
			},
			expected: "",
		},
		{
			name: "root_ver",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0, Meta: map[string]string{"version": "root_ver"}},
			},
			expected: "root_ver",
		},
		{
			name: "version",
			in: pb.Trace{
				&pb.Span{SpanID: 24, ParentID: 5, Meta: map[string]string{"version": "version"}},
				&pb.Span{ParentID: 0},
			},
			expected: "version",
		},
	}

	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, GetAppVersionFromTrace(traceutil.GetRoot(tc.in), &pb.TraceChunk{Spans: tc.in}))
		})
	}
}
