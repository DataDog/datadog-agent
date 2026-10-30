// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeJobNameForUrl(t *testing.T) {
	tests := []struct {
		name     string
		jobName  string
		expected string
	}{
		{name: "simple", jobName: "testJobName", expected: "testJobName"},
		{name: "with spaces", jobName: "test Job Name", expected: "test%20Job%20Name"},
		{name: "with slashes", jobName: "test/with/slashes", expected: "test/with/slashes"},
		{name: "with dots inside segment", jobName: "release..v1/.hidden/a.b", expected: "release..v1/.hidden/a.b"},
		{name: "with encoded traversal", jobName: "%2e%2e/scriptText", expected: "%252e%252e/scriptText"},
		{name: "with query and fragment", jobName: "job?x=1#frag", expected: "job%3Fx=1%23frag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeJobNameForUrl(tt.jobName)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestEncodeJobNameForUrlRejectsInvalidNames(t *testing.T) {
	for _, jobName := range []string{
		"",
		".",
		"..",
		"../scriptText",
		"x/../../scriptText",
		"folder/..",
		"./job",
		"folder/./job",
	} {
		t.Run(jobName, func(t *testing.T) {
			_, err := encodeJobNameForUrl(jobName)
			assert.Error(t, err)
		})
	}
}

func TestBuildJobURL(t *testing.T) {
	jobURL, err := buildJobURL("https://jenkins.example.com", BuildJobInputs{JobName: "folder/my job"})
	require.NoError(t, err)
	assert.Equal(t, "https://jenkins.example.com/job/folder/my%20job/build?delay=0sec", jobURL.String())

	jobURL, err = buildJobURL("https://jenkins.example.com", BuildJobInputs{
		JobName: "my-job",
		BuildWithParameters: &BuildWithParameters{
			Option:     WithParameters,
			Parameters: map[string]string{"branch": "main"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://jenkins.example.com/job/my-job/buildWithParameters?branch=main&delay=0sec", jobURL.String())

	_, err = buildJobURL("https://jenkins.example.com", BuildJobInputs{JobName: "x/../../scriptText"})
	assert.Error(t, err)
}
