// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	scrubChildEnv    = "ATEL_SCRUB_TAGS_CHILD"
	scrubChildMarker = "ATEL_SCRUB_TAGS_CHILD_RAN:"
)

// runScrubChild re-executes the test binary running only the named helper.
// scrubber.AddStrippedKeys is process-wide and irreversible, so the real
// scrubber must never be configured in the parent test process. The helper
// prints a marker line, because a -test.run that matches nothing exits 0.
func runScrubChild(t *testing.T, helper string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+helper+"$", "-test.v")
	cmd.Env = append(childBaseEnv(), scrubChildEnv+"="+helper)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "child process output:\n%s", out)
	require.Contains(t, string(out), scrubChildMarker+helper, "child did not run the helper:\n%s", out)
}

// childBaseEnv is the parent environment without the variables that make a
// test wrapper redirect the child to another test or to the parent's report.
func childBaseEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "TESTBRIDGE_TEST_ONLY="),
			strings.HasPrefix(kv, "GO_TEST_WRAP"),
			strings.HasPrefix(kv, "XML_OUTPUT_FILE="):
			continue
		}
		env = append(env, kv)
	}
	return env
}

// enterScrubChild skips unless running as the child and registers the key
// "tags" as stripped, as the secrets resolver does for a config key named
// "tags" holding a secret handle.
func enterScrubChild(t *testing.T, helper string) {
	t.Helper()
	if os.Getenv(scrubChildEnv) != helper {
		t.Skip("runs only as a child process of its parent test")
	}
	scrubber.AddStrippedKeys([]string{"tags"})
	// Printed unconditionally so the parent can prove the helper ran.
	println(scrubChildMarker + helper)
}

func TestSendLogsBatch_TagsSurviveKeyBasedScrubbing(t *testing.T) {
	runScrubChild(t, "TestSendLogsBatchTagsScrubHelper")
}

func TestSendLogsBatchTagsScrubHelper(t *testing.T) {
	enterScrubChild(t, "TestSendLogsBatchTagsScrubHelper")

	const tags = "git.commit.sha:abc,agent.flavor:x"
	cl := &captureClient{}
	s := newTestSender(t, cl)
	require.NoError(t, s.sendLogsBatch(context.Background(), []Log{
		{Level: LogLevelError, Tags: tags, Count: 1, TracerTime: 1},
	}))

	_, bodies := cl.snapshot()
	require.Len(t, bodies, 1)
	var got struct {
		Payload struct {
			Logs []Log `json:"logs"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(bodies[0], &got), "body: %s", bodies[0])
	require.Len(t, got.Payload.Logs, 1)
	assert.Equal(t, tags, got.Payload.Logs[0].Tags)
}

func TestSendPayloadMetricTags_TagsSurviveKeyBasedScrubbing(t *testing.T) {
	runScrubChild(t, "TestSendPayloadMetricTagsScrubHelper")
}

func TestSendPayloadMetricTagsScrubHelper(t *testing.T) {
	enterScrubChild(t, "TestSendPayloadMetricTagsScrubHelper")

	cl := &captureClient{}
	s := newTestSender(t, cl)
	payload := Payload{
		APIVersion:  "v2",
		RequestType: "agent-metrics",
		Host:        HostPayload{Hostname: "x"},
		Payload: AgentMetricsPayload{
			Message: "test",
			Metrics: map[string]interface{}{
				"foo.bar": MetricPayload{
					Value: 1,
					Type:  "counter",
					Tags:  map[string]interface{}{"git.commit.sha": "abc", "env": "test"},
				},
			},
		},
	}
	require.NoError(t, s.sendPayload(context.Background(), payload, "agent-metrics"))

	_, bodies := cl.snapshot()
	require.Len(t, bodies, 1)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(bodies[0], &got), "body: %s", bodies[0])
	inner, ok := got["payload"].(map[string]interface{})
	require.True(t, ok, "payload: %s", bodies[0])
	metrics, ok := inner["metrics"].(map[string]interface{})
	require.True(t, ok, "metrics: %s", bodies[0])
	metric, ok := metrics["foo.bar"].(map[string]interface{})
	require.True(t, ok, "metric: %s", bodies[0])
	tags, ok := metric["tags"].(map[string]interface{})
	require.True(t, ok, "tags must stay a JSON object, got %#v", metric["tags"])
	assert.Equal(t, map[string]interface{}{"git.commit.sha": "abc", "env": "test"}, tags)
}

func TestGetAsJSONMetricTags_TagsSurviveKeyBasedScrubbing(t *testing.T) {
	runScrubChild(t, "TestGetAsJSONMetricTagsScrubHelper")
}

func TestGetAsJSONMetricTagsScrubHelper(t *testing.T) {
	enterScrubChild(t, "TestGetAsJSONMetricTagsScrubHelper")

	c := `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.bar
                preserve_tags:
                  - env
                  - sha
    `
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "bar", []string{"env", "sha"}, "")
	counter.AddWithTags(10, map[string]string{"env": "test", "sha": "abc"})

	s := makeSenderImpl(t, nil, c)
	a := getTestAtel(t, tel, c, s, nil, newRunnerMock())
	require.True(t, a.enabled)

	out, err := a.getAsJSON()
	require.NoError(t, err)
	var top map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &top), "output: %s", out)

	// A single metric context yields one agent-metrics payload, possibly
	// wrapped in a message-batch; accept both shapes.
	var inners []interface{}
	if batch, ok := top["payload"].([]interface{}); ok {
		for _, p := range batch {
			if m, ok := p.(map[string]interface{}); ok {
				inners = append(inners, m["payload"])
			}
		}
	} else {
		inners = append(inners, top["payload"])
	}

	found := false
	for _, in := range inners {
		inner, ok := in.(map[string]interface{})
		if !ok {
			continue
		}
		metrics, _ := inner["metrics"].(map[string]interface{})
		metric, ok := metrics["bar.bar"].(map[string]interface{})
		if !ok {
			continue
		}
		found = true
		tags, ok := metric["tags"].(map[string]interface{})
		require.True(t, ok, "tags must stay a JSON object, got %#v", metric["tags"])
		assert.Equal(t, "test", tags["env"])
		assert.Equal(t, "abc", tags["sha"])
	}
	require.True(t, found, "bar.bar metric not found in output: %s", out)
	assert.False(t, strings.Contains(string(out), `"********"`), "output: %s", out)
}

// The shield and restore steps need no global scrubber state, so they run in
// the parent process.
func TestScrubKeepingTags_KeyShapes(t *testing.T) {
	t.Run("compact and indented key forms", func(t *testing.T) {
		for _, in := range []string{`{"tags":"a:b"}`, `{"tags" : "a:b"}`, "{\n  \"tags\": \"a:b\"\n}"} {
			out, err := scrubKeepingTags([]byte(in))
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(out, &got), string(out))
			assert.Equal(t, "a:b", got["tags"], string(out))
			assert.NotContains(t, string(out), "shielded")
		}
	})
	t.Run("the word tags as a value or inside a string value is untouched", func(t *testing.T) {
		in := `{"name":"tags","note":"{\"tags\": 1}"}`
		out, err := scrubKeepingTags([]byte(in))
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got), string(out))
		assert.Equal(t, "tags", got["name"])
		assert.Equal(t, `{"tags": 1}`, got["note"])
	})
	t.Run("a key that only ends with tags is not shielded", func(t *testing.T) {
		out, err := scrubKeepingTags([]byte(`{"host_tags":"a:b","tags":"c:d"}`))
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got), string(out))
		assert.Equal(t, "a:b", got["host_tags"])
		assert.Equal(t, "c:d", got["tags"])
	})
	t.Run("a payload already using the temporary key name keeps both keys", func(t *testing.T) {
		for _, in := range []string{
			`{"tags_shielded_from_key_scrub":"keep","tags":"a:b"}`,
			`{"tags":"a:b","tags_shielded_from_key_scrub":"keep","tags_shielded_from_key_scrub_1":"also"}`,
		} {
			out, err := scrubKeepingTags([]byte(in))
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(out, &got), string(out))
			assert.Equal(t, "a:b", got["tags"], string(out))
			assert.Equal(t, "keep", got["tags_shielded_from_key_scrub"], string(out))
			assert.Equal(t, 1, strings.Count(string(out), `"tags":`), string(out))
		}
	})
	t.Run("invalid JSON does not fail", func(t *testing.T) {
		_, err := scrubKeepingTags([]byte(`not json "tags":`))
		assert.NoError(t, err)
	})
}
