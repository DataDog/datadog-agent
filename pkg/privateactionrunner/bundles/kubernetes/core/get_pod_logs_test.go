// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPodLogsRegistered(t *testing.T) {
	_, ok := NewKubernetesCore().GetAction("getPodLogs").(*GetPodLogsHandler)
	require.True(t, ok)
}

func TestGetPodLogsInputsValidate(t *testing.T) {
	positive := int64(1)
	zero := int64(0)
	negative := int64(-1)
	tooLarge := maxPodLogsBytes + 1
	sinceTime := metav1.Now()

	tests := []struct {
		name    string
		inputs  GetPodLogsInputs
		wantErr string
	}{
		{name: "valid", inputs: GetPodLogsInputs{Name: "pod", Namespace: "default"}},
		{name: "missing name", inputs: GetPodLogsInputs{Namespace: "default"}, wantErr: "pod name is required"},
		{name: "missing namespace", inputs: GetPodLogsInputs{Name: "pod"}, wantErr: "pod namespace is required"},
		{
			name: "conflicting since selectors",
			inputs: GetPodLogsInputs{
				Name: "pod", Namespace: "default", SinceSeconds: &positive, SinceTime: &sinceTime,
			},
			wantErr: "only one of sinceSeconds and sinceTime",
		},
		{name: "zero since seconds", inputs: GetPodLogsInputs{Name: "pod", Namespace: "default", SinceSeconds: &zero}, wantErr: "sinceSeconds must be positive"},
		{name: "negative tail lines", inputs: GetPodLogsInputs{Name: "pod", Namespace: "default", TailLines: &negative}, wantErr: "tailLines must not be negative"},
		{name: "zero byte limit", inputs: GetPodLogsInputs{Name: "pod", Namespace: "default", LimitBytes: &zero}, wantErr: "limitBytes must be positive"},
		{name: "excessive byte limit", inputs: GetPodLogsInputs{Name: "pod", Namespace: "default", LimitBytes: &tooLarge}, wantErr: "limitBytes must not exceed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.inputs.validate()
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestGetPodLogsOptions(t *testing.T) {
	sinceSeconds := int64(60)
	tailLines := int64(100)
	limitBytes := int64(1024)
	inputs := GetPodLogsInputs{
		Container:    "agent",
		Previous:     true,
		SinceSeconds: &sinceSeconds,
		Timestamps:   true,
		TailLines:    &tailLines,
		LimitBytes:   &limitBytes,
	}

	options := inputs.options()
	require.Equal(t, inputs.Container, options.Container)
	require.Equal(t, inputs.Previous, options.Previous)
	require.Equal(t, inputs.Timestamps, options.Timestamps)
	require.Equal(t, inputs.SinceSeconds, options.SinceSeconds)
	require.Equal(t, inputs.TailLines, options.TailLines)
	require.Equal(t, inputs.LimitBytes, options.LimitBytes)

	defaultOptions := (GetPodLogsInputs{}).options()
	require.NotNil(t, defaultOptions.LimitBytes)
	require.Equal(t, maxPodLogsBytes, *defaultOptions.LimitBytes)
}

func TestReadPodLogs(t *testing.T) {
	t.Run("returns and closes the stream", func(t *testing.T) {
		reader := &trackingReadCloser{Reader: strings.NewReader("first\nsecond\n")}
		logs, err := readPodLogs(context.Background(), func(context.Context) (io.ReadCloser, error) {
			return reader, nil
		})
		require.NoError(t, err)
		require.Equal(t, "first\nsecond\n", logs)
		require.True(t, reader.closed)
	})

	t.Run("propagates stream errors", func(t *testing.T) {
		wantErr := errors.New("stream failed")
		_, err := readPodLogs(context.Background(), func(context.Context) (io.ReadCloser, error) {
			return nil, wantErr
		})
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("rejects oversized output", func(t *testing.T) {
		_, err := readPodLogs(context.Background(), func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(io.LimitReader(zeroReader{}, maxPodLogsBytes+1)), nil
		})
		require.ErrorContains(t, err, "output limit")
	})
}

func TestGetPodLogsMaskSequences(t *testing.T) {
	config := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"logs_config.processing_rules": []map[string]interface{}{
			{
				"type":                "mask_sequences",
				"name":                "mask_token",
				"pattern":             `token=[^[:space:]]+`,
				"replace_placeholder": "token=[MASKED]",
			},
			{
				"type":    "exclude_at_match",
				"name":    "unrelated_rule",
				"pattern": "drop this",
			},
		},
	})
	handler := newGetPodLogsHandler(config)

	logs, err := handler.maskSequences("first token=secret\ndrop this\nsecond token=another-secret")
	require.NoError(t, err)
	require.Equal(t, "first token=[MASKED]\ndrop this\nsecond token=[MASKED]", logs)
}

func TestGetPodLogsMaskSequencesRejectsInvalidRules(t *testing.T) {
	config := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"logs_config.processing_rules": []map[string]interface{}{
			{"type": "mask_sequences", "name": "invalid", "pattern": "("},
		},
	})

	_, err := newGetPodLogsHandler(config).maskSequences("token=secret")
	require.ErrorContains(t, err, "could not load global log processing rules")
}

func TestGetPodLogsMaskSequencesEnforcesOutputLimit(t *testing.T) {
	config := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"logs_config.processing_rules": []map[string]interface{}{
			{
				"type":                "mask_sequences",
				"name":                "expand",
				"pattern":             "x",
				"replace_placeholder": "xx",
			},
		},
	})

	_, err := newGetPodLogsHandler(config).maskSequences(strings.Repeat("x", int(maxPodLogsBytes)))
	require.ErrorContains(t, err, "output limit")
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
