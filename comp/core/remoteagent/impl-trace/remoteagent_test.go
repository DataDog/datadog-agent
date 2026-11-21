// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package traceimpl

import (
	"context"
	"expvar"
	"strings"
	"testing"

	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetFlareFiles(t *testing.T) {
	// expvar keys are process-global and publishing an existing key panics, so
	// use keys reserved for this test.
	expvar.Publish("datadog_agent_remoteagent_impl_trace_test", expvar.Func(func() interface{} {
		return map[string]int{"value": 42}
	}))
	// A value that fails JSON marshaling: expvar.Func renders it as an empty
	// string, which is not valid JSON, covering the raw-string fallback.
	expvar.Publish("datadog_agent_remoteagent_impl_trace_test_bad", expvar.Func(func() interface{} {
		return make(chan int)
	}))

	impl := &remoteagentImpl{}
	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})
	if err != nil {
		t.Fatalf("GetFlareFiles returned an error: %v", err)
	}

	if len(resp.Files) != 1 {
		t.Fatalf("expected exactly one flare file, got %d", len(resp.Files))
	}
	data, ok := resp.Files["expvar.yaml"]
	if !ok {
		t.Fatalf("expected an expvar.yaml file, got files %v", keys(resp.Files))
	}

	content := string(data)
	if !strings.Contains(content, "datadog_agent_remoteagent_impl_trace_test:") {
		t.Errorf("expvar snapshot is missing the published key, got:\n%s", content)
	}
	if !strings.Contains(content, "value: 42") {
		t.Errorf("expvar snapshot is missing the published value, got:\n%s", content)
	}
	if !strings.Contains(content, "datadog_agent_remoteagent_impl_trace_test_bad: \"\"") {
		t.Errorf("expvar snapshot is missing the unmarshalable variable's raw value, got:\n%s", content)
	}
}

func keys(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for k := range files {
		out = append(out, k)
	}
	return out
}
