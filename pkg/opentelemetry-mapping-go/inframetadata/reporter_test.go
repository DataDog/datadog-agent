// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package inframetadata

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	conventions "go.opentelemetry.io/otel/semconv/v1.18.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/internal/testutils"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/payload"
)

func TestHasHostMetadata(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		ok    bool
		err   string
	}{
		{
			name: "wrong type for datadog.host.use_as_metadata",
			attrs: map[string]any{
				AttributeDatadogHostUseAsMetadata: "a string",
			},
			err: "\"datadog.host.use_as_metadata\" has type \"Str\", expected \"Bool\"",
		},
		{
			name:  "no datadog.host.use_as_metadata",
			attrs: map[string]any{},
			ok:    shouldUseByDefault,
		},
		{
			name: "datadog.host.use_as_metadata = true",
			attrs: map[string]any{
				AttributeDatadogHostUseAsMetadata: true,
			},
			ok: true,
		},
		{
			name: "datadog.host.use_as_metadata = false",
			attrs: map[string]any{
				AttributeDatadogHostUseAsMetadata: false,
			},
			ok: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := hasHostMetadata(testutils.NewResourceFromMap(t, tt.attrs))
			if tt.err != "" {
				assert.EqualError(t, err, tt.err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.ok, ok)
			}
		})
	}

}

var _ Pusher = (*pusher)(nil)

type pusher struct {
	md payload.HostMetadata
	ch chan struct{}
}

func (p *pusher) Push(_ context.Context, md payload.HostMetadata) error {
	p.md = md
	close(p.ch)
	return errors.New("network error")
}

func TestRun(t *testing.T) {
	p := &pusher{ch: make(chan struct{})}
	core, observed := observer.New(zapcore.WarnLevel)
	r, err := NewReporter(zap.New(core), p, 50*time.Millisecond)
	require.NoError(t, err)

	ch := make(chan struct{})
	go func() {
		require.NoError(t, r.Run(context.Background()))
		close(ch)
	}()

	err = r.ConsumeResource(testutils.NewResourceFromMap(t, map[string]any{
		AttributeDatadogHostUseAsMetadata:    true,
		string(conventions.CloudProviderKey): conventions.CloudProviderAWS.Value.AsString(),
		string(conventions.HostIDKey):        "host-1-hostid",
		string(conventions.HostNameKey):      "host-1-hostname",
		string(conventions.OSDescriptionKey): true,
		string(conventions.HostArchKey):      conventions.HostArchAMD64.Value.AsString(),
	}))
	assert.EqualError(t, err, "\"os.description\" has type \"Bool\", expected type \"Str\" instead")

	err = r.ConsumeResource(testutils.NewResourceFromMap(t, map[string]any{}))
	assert.NoError(t, err)

	// wait until Push has been called once before stopping
	<-p.ch
	r.Stop()
	// wait until Reporter has stopped
	<-ch
	assert.Equal(t, p.md.Meta.Hostname, "host-1-hostid")
	assert.Contains(t, p.md.Tags.OTel, "cloud_provider:aws")
	logs := observed.AllUntimed()
	assert.Len(t, logs, 1)
	assert.Equal(t, logs[0].Message, "Failed to send host metadata")
}

type channelPusher struct {
	out chan payload.HostMetadata
}

var _ Pusher = (*channelPusher)(nil)

func (p *channelPusher) Push(_ context.Context, md payload.HostMetadata) error {
	p.out <- md
	return nil
}

func TestHostMapUpdateRace(t *testing.T) {
	p := &channelPusher{
		out: make(chan payload.HostMetadata),
	}
	r, err := NewReporter(zaptest.NewLogger(t), p, time.Second)
	require.NoError(t, err)

	go func() {
		md := <-p.out
		// Repeatedly iterate over the first payload
		for range 1000 {
			_, _ = json.Marshal(md)
		}
	}()

	for range 1000 {
		// Repeatedly modify the payload in the host map
		err = r.ConsumeResource(testutils.NewResourceFromMap(t, map[string]any{
			AttributeDatadogHostUseAsMetadata: true,
			string(conventions.HostNameKey):   "test-hostname",
		}))
		assert.NoError(t, err)
	}
	close(p.out)
}
