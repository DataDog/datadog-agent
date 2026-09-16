// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/agent-payload/v5/pb"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestResolveMalformedTagFilterRecordsMessage(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"no_colon_here"}},
	})
	msg := newMessage([]byte("hello"), source, message.StatusInfo)

	p.resolveTagFilter(msg)

	_, ok := source.TagFilter()
	assert.True(t, ok, "the source must still resolve despite the malformed pattern")

	found := false
	for _, m := range source.Messages.GetMessages() {
		if strings.Contains(m, "no_colon_here") {
			found = true
		}
	}
	assert.True(t, found, "expected a message naming the rejected pattern, got %v", source.Messages.GetMessages())
}

func TestResolveSourceTagFilterConcurrentCallsConverge(t *testing.T) {
	global, _ := tagfilter.Compile(nil, []string{"team:*"})
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"pod_name:*"}},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ResolveSourceTagFilter(global, source)
		}()
	}
	wg.Wait()

	resolved, ok := source.TagFilter()
	assert.True(t, ok)
	assert.NotNil(t, resolved)
	assert.NotNil(t, source.GetInfo("Tag Filters"))
}

func TestEncodersApplySourceFilter(t *testing.T) {
	logsConfig := &config.LogsConfig{
		Source:         "mysource",
		SourceCategory: "cat",
		Tags:           []string{"keep:me", "drop:me"},
	}
	source := sources.NewLogSource("", logsConfig)
	f, _ := tagfilter.Compile(nil, []string{"drop:*", "sourcecategory:*", "source:*"})
	source.SetTagFilterIfUnset(tagfilter.NewScoped(f, nil))

	buildMsg := func() *message.Message {
		msg := newMessage([]byte("message"), source, message.StatusInfo)
		msg.State = message.StateRendered
		msg.Origin.LogSource = source
		return msg
	}

	t.Run("json", func(t *testing.T) {
		msg := buildMsg()
		assert.NoError(t, JSONEncoder.Encode(msg, "host"))
		var decoded jsonPayload
		assert.NoError(t, json.Unmarshal(msg.GetContent(), &decoded))
		assert.NotContains(t, decoded.Tags, "drop:me")
		assert.Contains(t, decoded.Tags, "keep:me")
	})

	t.Run("proto", func(t *testing.T) {
		msg := buildMsg()
		assert.NoError(t, ProtoEncoder.Encode(msg, "host"))
		log := &pb.Log{}
		assert.NoError(t, log.Unmarshal(msg.GetContent()))
		assert.NotContains(t, log.Tags, "drop:me")
		assert.Contains(t, log.Tags, "keep:me")
	})

	t.Run("raw", func(t *testing.T) {
		msg := buildMsg()
		assert.NoError(t, RawEncoder.Encode(msg, "host"))
		content := string(msg.GetContent())
		assert.Contains(t, content, "ddsource=\"mysource\"")
		assert.NotContains(t, content, "ddsourcecategory")
		assert.NotContains(t, content, "drop:me")
		assert.Contains(t, content, "keep:me")
	})
}

func TestResolveSourceTagFilterNilConfig(t *testing.T) {
	global, _ := tagfilter.Compile(nil, []string{"container_id:*"})

	ResolveSourceTagFilter(global, sources.NewLogSource("nil-config", nil))
	ResolveSourceTagFilter(global, nil)
}
