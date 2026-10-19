// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatformnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestExternalHostTags(t *testing.T) {
	host1 := "localhost"
	host2 := "127.0.0.1"
	sourceType := "vsphere"
	tags1 := []string{"foo", "bar"}
	tags2 := []string{"baz"}
	eTags1 := externalhost.ExternalTags{sourceType: tags1}
	eTags2 := externalhost.ExternalTags{sourceType: tags2}
	externalhost.SetExternalTags(host1, sourceType, tags1)
	externalhost.SetExternalTags(host2, sourceType, tags2)

	hostname, _ := hostnameinterface.NewMock("my-hostname")
	c := newCollector(dependencies{
		Lc:               compdef.NewTestLifecycle(t),
		Config:           config.NewMockWithOverrides(t, map[string]interface{}{"check_cancel_timeout": 500 * time.Millisecond}),
		Log:              logmock.New(t),
		HaAgent:          haagentmock.NewMockHaAgent(),
		HealthPlatform:   healthplatformnoopimpl.NewNoopComponent(),
		Hostname:         hostname,
		SenderManager:    aggregator.NewNoOpSenderManager(),
		MetricSerializer: option.None[serializer.MetricSerializer](),
		AgentTelemetry:   option.None[agenttelemetry.Component](),
	})

	pl := c.GetPayload(context.Background())
	hpl := pl.ExternalhostTags
	assert.Len(t, hpl, 2)
	for _, elem := range hpl {
		if elem[0] == host1 {
			assert.Equal(t, eTags1, elem[1])
		} else if elem[0] == host2 {
			assert.Equal(t, eTags2, elem[1])
		} else {
			assert.Fail(t, "Unexpected value for hostname: %s", elem[0])
		}
	}
}

func TestCollectTags(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   []string
	}{
		{
			name: "list of tags",
			config: `
max_returned_metrics: 50000
tags:
  - foo:bar
  - baz:qux
`,
			want: []string{"foo:bar", "baz:qux"},
		},
		{
			name: "array of tags",
			config: `
max_returned_metrics: 50000
tags: [foo:bar, baz:qux]
`,
			want: []string{"foo:bar", "baz:qux"},
		},
		{
			name: "scalar value",
			config: `
max_returned_metrics: 50000
tags: "foo:bar"
`,
			want: []string{"foo:bar"},
		},
		{
			name:   "empty",
			config: ``,
			want:   []string{},
		},
		{
			name: "below root",
			config: `
max_returned_metrics: 50000
level: 
  tags: "foo:bar"
`,
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := collectTags(test.config)
			assert.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestTruncateCheckError(t *testing.T) {
	short := "short error"
	assert.Equal(t, short, truncateCheckError(short))

	exact := strings.Repeat("a", maxCheckErrorLength)
	assert.Equal(t, exact, truncateCheckError(exact))

	long := "Traceback (most recent call last):\n" +
		strings.Repeat("another command is already in progress\n", maxCheckErrorLength) +
		"psycopg.OperationalError: connection is bad"
	got := truncateCheckError(long)
	assert.LessOrEqual(t, len(got), maxCheckErrorLength)
	assert.True(t, strings.HasPrefix(got, "Traceback (most recent call last):\n"))
	assert.True(t, strings.HasSuffix(got, "psycopg.OperationalError: connection is bad"))
	assert.Contains(t, got, " bytes truncated ...]")

	// must not split a multi-byte character, at either end of the dropped part: try both offset parities
	for _, prefix := range []string{"", "a"} {
		multi := prefix + strings.Repeat("é", maxCheckErrorLength)
		got := truncateCheckError(multi)
		assert.LessOrEqual(t, len(got), maxCheckErrorLength)
		assert.True(t, utf8.ValidString(got), "prefix %q", prefix)
	}
}

func TestTruncateLoaderErrors(t *testing.T) {
	longMsg := strings.Repeat("another command is already in progress\n", maxCheckErrorLength)
	errs := map[string]string{"python": longMsg, "core": "check not found"}

	got := truncateLoaderErrors(errs)
	assert.Equal(t, "check not found", got["core"])
	assert.LessOrEqual(t, len(got["python"]), maxCheckErrorLength)
	assert.Equal(t, longMsg, errs["python"], "input map must not be modified")

	jsonErrs, err := json.Marshal(got)
	require.NoError(t, err)
	var decoded map[string]string
	require.NoError(t, json.Unmarshal(jsonErrs, &decoded))
	assert.Equal(t, got, decoded)
}

func TestGetPayloadTruncatesCheckMessages(t *testing.T) {
	expvars.Reset()
	t.Cleanup(expvars.Reset)

	longMsg := strings.Repeat("another command is already in progress\n", maxCheckErrorLength)
	errCheck := &TestCheck{name: "error_check"}
	warnCheck := &TestCheck{name: "warning_check"}
	expvars.AddCheckStats(errCheck, 0, errors.New(longMsg), nil, checkstats.SenderStats{}, haagentmock.NewMockHaAgent())
	expvars.AddCheckStats(warnCheck, 0, nil, []error{errors.New(longMsg)}, checkstats.SenderStats{}, haagentmock.NewMockHaAgent())

	pl := newUnstartedTestCollector(t, nil).GetPayload(context.Background())

	statuses := map[checkid.ID][]interface{}{}
	for _, entry := range pl.AgentChecks {
		status := entry.([]interface{})
		if id, ok := status[2].(checkid.ID); ok {
			statuses[id] = status
		}
	}
	require.Contains(t, statuses, errCheck.ID())
	require.Contains(t, statuses, warnCheck.ID())

	assert.Equal(t, "ERROR", statuses[errCheck.ID()][3])
	assert.LessOrEqual(t, len(statuses[errCheck.ID()][4].(string)), maxCheckErrorLength)

	assert.Equal(t, "WARNING", statuses[warnCheck.ID()][3])
	warnings := statuses[warnCheck.ID()][4].([]string)
	require.Len(t, warnings, 1)
	assert.LessOrEqual(t, len(warnings[0]), maxCheckErrorLength)
}
