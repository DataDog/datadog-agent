// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
)

type logsDDTagsTest struct {
	name             string
	logsTagsAsDDTags bool
	taggerTags       []string
	// existingLogRecordDdtags, if non-empty, seeds a `ddtags` log record
	// attribute before the processor runs, to verify merge (not overwrite)
	// semantics.
	existingLogRecordDdtags string
	outResourceAttributes   map[string]any
	outLogRecordDdtags      string
}

// logsDDTagsTests exercises the LogsTagsAsDDTags option: when enabled, custom
// tagger tags become a `ddtags` log record attribute (a real Datadog log tag)
// instead of a resource attribute (a log attribute). Known conventions and
// USM keys always stay as resource attributes either way, since the Datadog
// logs intake already promotes them into tags on its own.
var logsDDTagsTests = []logsDDTagsTest{
	{
		name:             "disabled (default), custom tag stays a resource attribute",
		logsTagsAsDDTags: false,
		taggerTags:       []string{"team:platform"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"team":         "platform",
		},
		outLogRecordDdtags: "",
	},
	{
		name:             "enabled, custom tag becomes ddtags instead of a resource attribute",
		logsTagsAsDDTags: true,
		taggerTags:       []string{"team:platform"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
		},
		outLogRecordDdtags: "team:platform",
	},
	{
		name:             "enabled, DD-convention key (pod_name) stays a resource attribute",
		logsTagsAsDDTags: true,
		taggerTags:       []string{"pod_name:foo"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"pod_name":     "foo",
		},
		outLogRecordDdtags: "",
	},
	{
		// `kube_service` lives in attributes.KubernetesDDTags but not in
		// knownConventionKeys. Regression guard for the bug where such keys
		// were diverted into ddtags, even though the OTLP logs translator
		// (attributes.TagsFromAttributes) already promotes them from
		// resource attributes into tags downstream.
		name:             "enabled, KubernetesDDTags-only key (kube_service) stays a resource attribute",
		logsTagsAsDDTags: true,
		taggerTags:       []string{"kube_service:mysvc"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"kube_service": "mysvc",
		},
		outLogRecordDdtags: "",
	},
	{
		name:             "enabled, USM tag (service) flows through USM path, not ddtags",
		logsTagsAsDDTags: true,
		taggerTags:       []string{"service:svc"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"service.name": "svc",
		},
		outLogRecordDdtags: "",
	},
	{
		name:                    "enabled, custom tag is merged with pre-existing ddtags log record attribute",
		logsTagsAsDDTags:        true,
		taggerTags:              []string{"team:platform"},
		existingLogRecordDdtags: "user_set:tag",
		outResourceAttributes: map[string]any{
			"container.id": "test",
		},
		outLogRecordDdtags: "user_set:tag,team:platform",
	},
	{
		name:                    "enabled, no custom tags leaves pre-existing ddtags untouched",
		logsTagsAsDDTags:        true,
		taggerTags:              []string{"pod_name:foo"},
		existingLogRecordDdtags: "user_set:tag",
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"pod_name":     "foo",
		},
		outLogRecordDdtags: "user_set:tag",
	},
}

func TestLogsTagsAsDDTags(t *testing.T) {
	for _, tt := range logsDDTagsTests {
		t.Run(tt.name, func(t *testing.T) {
			next := new(consumertest.LogsSink)
			cfg := &Config{
				Cardinality:      types.LowCardinality,
				LogsTagsAsDDTags: tt.logsTagsAsDDTags,
			}
			tc := testutil.NewTestTaggerClient()
			tc.TagMap["container_id://test"] = tt.taggerTags

			factory := NewFactoryForAgent(tc, func(_ context.Context) (string, error) {
				return "test-host", nil
			})
			flp, err := factory.CreateLogs(
				context.Background(),
				processortest.NewNopSettings(Type),
				cfg,
				next,
			)
			require.NoError(t, err)
			require.NotNil(t, flp)

			ctx := context.Background()
			require.NoError(t, flp.Start(ctx, nil))

			ld := plog.NewLogs()
			rl := ld.ResourceLogs().AppendEmpty()
			//nolint:errcheck
			rl.Resource().Attributes().FromRaw(map[string]any{"container.id": "test"})
			lr := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
			if tt.existingLogRecordDdtags != "" {
				lr.Attributes().PutStr("ddtags", tt.existingLogRecordDdtags)
			}

			cErr := flp.ConsumeLogs(ctx, ld)
			assert.NoError(t, cErr)
			assert.NoError(t, flp.Shutdown(ctx))

			require.Len(t, next.AllLogs(), 1)
			out := next.AllLogs()[0].ResourceLogs().At(0)
			assert.EqualValues(t, tt.outResourceAttributes, out.Resource().Attributes().AsRaw())

			outLr := out.ScopeLogs().At(0).LogRecords().At(0)
			ddtagsVal, ok := outLr.Attributes().Get("ddtags")
			if tt.outLogRecordDdtags == "" {
				assert.False(t, ok, "expected no ddtags log record attribute")
			} else {
				require.True(t, ok, "expected a ddtags log record attribute")
				assert.Equal(t, tt.outLogRecordDdtags, ddtagsVal.AsString())
			}
		})
	}
}
