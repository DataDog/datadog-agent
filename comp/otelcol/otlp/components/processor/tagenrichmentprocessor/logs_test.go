// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package tagenrichmentprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"
)

type logNameTest struct {
	name   string
	inLogs plog.Logs
	outLN  [][]string // output Log names per Resource
}

var (
	standardLogTests = []logNameTest{
		{
			name:   "emptyFilterInclude",
			inLogs: plog.NewLogs(),
			outLN:  [][]string{},
		},
	}
)

func TestTagEnrichmentLogProcessor(t *testing.T) {
	for _, test := range standardLogTests {
		t.Run(test.name, func(t *testing.T) {
			// next stores the results of the filter log processor
			next := new(consumertest.LogsSink)
			cfg := &Config{
				Logs: LogTagEnrichment{},
			}
			factory := NewFactory()
			flp, err := factory.CreateLogsProcessor(
				context.Background(),
				processortest.NewNopCreateSettings(),
				cfg,
				next,
			)
			assert.NotNil(t, flp)
			assert.NoError(t, err)

			caps := flp.Capabilities()
			assert.True(t, caps.MutatesData)
			ctx := context.Background()
			assert.NoError(t, flp.Start(ctx, nil))

			cErr := flp.ConsumeLogs(context.Background(), test.inLogs)
			assert.Nil(t, cErr)
			got := next.AllLogs()

			require.Len(t, got, 1)
			rLogs := got[0].ResourceLogs()
			assert.Equal(t, len(test.outLN), rLogs.Len())

			for i, wantOut := range test.outLN {
				gotLogs := rLogs.At(i).ScopeLogs().At(0).LogRecords()
				assert.Equal(t, len(wantOut), gotLogs.Len())
				for idx := range wantOut {
					val, ok := gotLogs.At(idx).Attributes().Get("name")
					require.True(t, ok)
					assert.Equal(t, wantOut[idx], val.AsString())
				}
			}
			assert.NoError(t, flp.Shutdown(ctx))
		})
	}
}
