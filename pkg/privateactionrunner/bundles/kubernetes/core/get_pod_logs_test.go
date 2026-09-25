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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPodLogsRegistered(t *testing.T) {
	if _, ok := NewKubernetesCore().GetAction("getPodLogs").(*GetPodLogsHandler); !ok {
		t.Fatal("getPodLogs is not registered with its handler")
	}
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
				if err != nil {
					t.Fatalf("validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validate() error = %v, want error containing %q", err, test.wantErr)
			}
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
	if options.Container != inputs.Container || options.Previous != inputs.Previous || options.Timestamps != inputs.Timestamps {
		t.Fatalf("options() did not preserve scalar inputs: %#v", options)
	}
	if options.SinceSeconds != inputs.SinceSeconds || options.TailLines != inputs.TailLines || options.LimitBytes != inputs.LimitBytes {
		t.Fatalf("options() did not preserve pointer inputs: %#v", options)
	}

	defaultOptions := (GetPodLogsInputs{}).options()
	if defaultOptions.LimitBytes == nil || *defaultOptions.LimitBytes != maxPodLogsBytes {
		t.Fatalf("options() default limit = %v, want %d", defaultOptions.LimitBytes, maxPodLogsBytes)
	}
}

func TestReadPodLogs(t *testing.T) {
	t.Run("returns and closes the stream", func(t *testing.T) {
		reader := &trackingReadCloser{Reader: strings.NewReader("first\nsecond\n")}
		logs, err := readPodLogs(context.Background(), func(context.Context) (io.ReadCloser, error) {
			return reader, nil
		})
		if err != nil {
			t.Fatalf("readPodLogs() error = %v", err)
		}
		if logs != "first\nsecond\n" {
			t.Fatalf("readPodLogs() = %q", logs)
		}
		if !reader.closed {
			t.Fatal("readPodLogs() did not close the stream")
		}
	})

	t.Run("propagates stream errors", func(t *testing.T) {
		wantErr := errors.New("stream failed")
		_, err := readPodLogs(context.Background(), func(context.Context) (io.ReadCloser, error) {
			return nil, wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("readPodLogs() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("rejects oversized output", func(t *testing.T) {
		_, err := readPodLogs(context.Background(), func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(io.LimitReader(zeroReader{}, maxPodLogsBytes+1)), nil
		})
		if err == nil || !strings.Contains(err.Error(), "output limit") {
			t.Fatalf("readPodLogs() error = %v, want output limit error", err)
		}
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
	if err != nil {
		t.Fatalf("maskSequences() error = %v", err)
	}
	want := "first token=[MASKED]\ndrop this\nsecond token=[MASKED]"
	if logs != want {
		t.Fatalf("maskSequences() = %q, want %q", logs, want)
	}
}

func TestGetPodLogsMaskSequencesRejectsInvalidRules(t *testing.T) {
	config := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"logs_config.processing_rules": []map[string]interface{}{
			{"type": "mask_sequences", "name": "invalid", "pattern": "("},
		},
	})

	_, err := newGetPodLogsHandler(config).maskSequences("token=secret")
	if err == nil || !strings.Contains(err.Error(), "could not load global log processing rules") {
		t.Fatalf("maskSequences() error = %v, want processing rules error", err)
	}
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
	if err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("maskSequences() error = %v, want output limit error", err)
	}
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
