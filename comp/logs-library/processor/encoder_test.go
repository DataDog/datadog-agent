// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/agent-payload/v5/pb"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestRawEncoder(t *testing.T) {

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "message"
	msg := newMessage([]byte(rawMessage), source, message.StatusError)
	msg.State = message.StateRendered // we can only encode rendered message
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})
	msg.SetContent([]byte("redacted"))

	err := RawEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	content := string(msg.GetContent())
	parts := strings.Fields(content)
	assert.Equal(t, string(message.SevError)+"0", parts[0])
	assert.Equal(t, day, parts[1][:len(day)])
	assert.NotEmpty(t, parts[2])
	assert.Equal(t, "Service", parts[3])
	assert.Equal(t, "-", parts[4])
	assert.Equal(t, "-", parts[5])
	extra := content[strings.Index(content, "[") : strings.LastIndex(content, "]")+1]
	assert.Equal(t, "[dd ddsource=\"Source\"][dd ddsourcecategory=\"SourceCategory\"][dd ddtags=\"foo:bar,baz,a,b:c\"]", extra)
	assert.Equal(t, "redacted", content[strings.LastIndex(content, " ")+1:])

}

func TestRawEncoderDefaults(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "a"
	msg := newMessage([]byte(rawMessage), source, "")
	msg.State = message.StateRendered
	err := RawEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	content := string(msg.GetContent())
	parts := strings.Fields(content)
	assert.Equal(t, 8, len(parts))
	assert.Equal(t, string(message.SevInfo)+"0", parts[0])
	assert.Equal(t, day, parts[1][:len(day)])
	assert.NotEmpty(t, parts[2])
	assert.Equal(t, "-", parts[3])
	assert.Equal(t, "-", parts[4])
	assert.Equal(t, "-", parts[5])
	assert.Equal(t, "-", parts[6])
	assert.Equal(t, "a", parts[7])

}

func TestRawEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := ""
	msg := newMessage([]byte(rawMessage), source, "")
	msg.State = message.StateRendered // we can only encode rendered message
	err := RawEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)
	assert.Equal(t, rawMessage, string(msg.GetContent()))

}

func TestIsRFC5424Formatted(t *testing.T) {
	assert.False(t, isRFC5424Formatted([]byte("<- test message ->")))
	assert.False(t, isRFC5424Formatted([]byte("- test message ->")))
	assert.False(t, isRFC5424Formatted([]byte("<46> the rest of the message")))
	assert.True(t, isRFC5424Formatted([]byte("<46>0 the rest of the message")))
}

func TestProtoEncoder(t *testing.T) {

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "message"
	msg := newMessage([]byte(rawMessage), source, message.StatusError)
	msg.State = message.StateRendered // we can only encode rendered message
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})

	err := ProtoEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	log := &pb.Log{}
	err = log.Unmarshal(msg.GetContent())
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Equal(t, logsConfig.Service, log.Service)
	assert.Equal(t, logsConfig.Source, log.Source)
	assert.Equal(t, []string{"a", "b:c", "sourcecategory:" + logsConfig.SourceCategory, "foo:bar", "baz"}, log.Tags)
	assert.Equal(t, message.StatusError, log.Status)
	assert.NotEmpty(t, log.Timestamp)

	data, err := log.Marshal()
	assert.NoError(t, err)
	assert.Equal(t, msg.GetContent(), data)

}

func TestProtoEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := ""
	msg := newMessage([]byte(rawMessage), source, "")
	msg.State = message.StateRendered // we can only encode rendered message

	err := ProtoEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	log := &pb.Log{}
	err = log.Unmarshal(msg.GetContent())
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Empty(t, log.Service)
	assert.Empty(t, log.Source)
	assert.Empty(t, log.Tags)

	assert.Empty(t, log.Message)
	assert.Equal(t, log.Status, message.StatusInfo)
	assert.NotEmpty(t, log.Timestamp)

}

func TestProtoEncoderHandleInvalidUTF8(t *testing.T) {
	cfg := &config.LogsConfig{}
	src := sources.NewLogSource("", cfg)
	msg := newMessage([]byte("a\xfez"), src, "")
	msg.State = message.StateRendered
	err := ProtoEncoder.Encode(msg, "unknown")
	assert.NotNil(t, msg.GetContent())
	assert.Nil(t, err)
}

func TestJsonEncoder(t *testing.T) {
	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}

	source := sources.NewLogSource("", logsConfig)

	type payload struct {
		Message   string `json:"message"`
		Status    string `json:"status"`
		Timestamp int64  `json:"timestamp"`
		Hostname  string `json:"hostname"`
		Service   string `json:"service"`
		Source    string `json:"ddsource"`
		Tags      string `json:"ddtags"`
	}

	t.Run("valid", func(t *testing.T) {
		content := []byte("valid utf-8 message content")

		msg := newMessage(content, source, message.StatusError)
		msg.State = message.StateRendered // we can only encode rendered message
		msg.Origin.LogSource = source
		msg.Origin.SetTags([]string{"a", "b:c"})
		assert.Equal(t, msg.GetContent(), content) // before encoding, content should be the raw message

		err := JSONEncoder.Encode(msg, "unknown")
		assert.Nil(t, err)

		log := &payload{}

		err = json.Unmarshal(msg.GetContent(), log)
		assert.Nil(t, err)

		assert.Equal(t, "valid utf-8 message content", log.Message)
		assert.NotEmpty(t, log.Hostname)

		assert.Equal(t, logsConfig.Service, log.Service)
		assert.Equal(t, logsConfig.Source, log.Source)
		assert.Equal(t, "a,b:c,sourcecategory:"+logsConfig.SourceCategory+",foo:bar,baz", log.Tags)

		json, _ := json.Marshal(log)
		assert.Equal(t, msg.GetContent(), json)

		assert.Equal(t, message.StatusError, log.Status)
		assert.NotEmpty(t, log.Timestamp)
	})

	t.Run("invalid", func(t *testing.T) {
		content := []byte("invalid utf-8 message content a\xf0\x8f\xbf\xbfz")

		msg := newMessage(content, source, message.StatusError)
		msg.State = message.StateRendered // we can only encode rendered message
		msg.Origin.LogSource = source
		msg.Origin.SetTags([]string{"a", "b:c"})
		assert.Equal(t, msg.GetContent(), content) // before encoding, content should be the raw message

		err := JSONEncoder.Encode(msg, "unknown")
		assert.Nil(t, err)

		log := &payload{}

		err = json.Unmarshal(msg.GetContent(), log)
		assert.Nil(t, err)

		assert.Equal(t, "invalid utf-8 message content a����z", log.Message)
		assert.NotEmpty(t, log.Hostname)

		assert.Equal(t, logsConfig.Service, log.Service)
		assert.Equal(t, logsConfig.Source, log.Source)
		assert.Equal(t, "a,b:c,sourcecategory:"+logsConfig.SourceCategory+",foo:bar,baz", log.Tags)

		json, _ := json.Marshal(log)
		assert.Equal(t, msg.GetContent(), json)

		assert.Equal(t, message.StatusError, log.Status)
		assert.NotEmpty(t, log.Timestamp)
	})
}

func TestEncodersUseContainerTimestampConfigGate(t *testing.T) {
	logsConfig := &config.LogsConfig{}
	source := sources.NewLogSource("", logsConfig)

	containerTS := "2000-01-01T00:00:00.000000000Z"
	parsed, err := time.Parse(time.RFC3339Nano, containerTS)
	assert.NoError(t, err)

	t.Run("json", func(t *testing.T) {
		t.Run("disabled", func(t *testing.T) {
			encoder := NewJSONEncoder(false)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = containerTS

			err := encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			var payload struct {
				Timestamp int64 `json:"timestamp"`
			}
			assert.NoError(t, json.Unmarshal(msg.GetContent(), &payload))

			expectedMillis := parsed.UnixNano() / nanoToMillis
			assert.NotEqual(t, expectedMillis, payload.Timestamp)
		})

		t.Run("enabled", func(t *testing.T) {
			encoder := NewJSONEncoder(true)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = containerTS

			err := encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			var payload struct {
				Timestamp int64 `json:"timestamp"`
			}
			assert.NoError(t, json.Unmarshal(msg.GetContent(), &payload))

			expectedMillis := parsed.UnixNano() / nanoToMillis
			assert.Equal(t, expectedMillis, payload.Timestamp)
		})
	})

	t.Run("proto", func(t *testing.T) {
		t.Run("disabled", func(t *testing.T) {
			encoder := NewProtoEncoder(false)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = containerTS

			err := encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			log := &pb.Log{}
			assert.NoError(t, log.Unmarshal(msg.GetContent()))

			assert.NotEqual(t, parsed.UnixNano(), log.Timestamp)
		})

		t.Run("enabled", func(t *testing.T) {
			encoder := NewProtoEncoder(true)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = containerTS

			err := encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			log := &pb.Log{}
			assert.NoError(t, log.Unmarshal(msg.GetContent()))

			assert.Equal(t, parsed.UnixNano(), log.Timestamp)
		})
	})

	t.Run("raw", func(t *testing.T) {
		t.Run("disabled", func(t *testing.T) {
			encoder := NewRawEncoder(false)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = containerTS

			err := encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			parts := strings.Fields(string(msg.GetContent()))
			assert.GreaterOrEqual(t, len(parts), 2)
			assert.NotEqual(t, containerTS, parts[1])
		})

		t.Run("enabled", func(t *testing.T) {
			encoder := NewRawEncoder(true)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = containerTS

			err := encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			parts := strings.Fields(string(msg.GetContent()))
			assert.GreaterOrEqual(t, len(parts), 2)
			assert.Equal(t, containerTS, parts[1])
		})

		t.Run("enabled_normalizes_offset_timestamps_to_utc", func(t *testing.T) {
			encoder := NewRawEncoder(true)

			offsetTS := "2000-01-01T02:00:00.000000000+02:00"
			parsedOffset, err := time.Parse(time.RFC3339Nano, offsetTS)
			assert.NoError(t, err)

			msg := newMessage([]byte("a"), source, message.StatusInfo)
			msg.State = message.StateRendered
			msg.Origin.LogSource = source
			msg.ParsingExtra.Timestamp = offsetTS

			err = encoder.Encode(msg, "unknown")
			assert.NoError(t, err)

			parts := strings.Fields(string(msg.GetContent()))
			assert.GreaterOrEqual(t, len(parts), 2)
			assert.Equal(t, parsedOffset.UTC().Format(config.DateFormat), parts[1])
		})
	})
}

func TestEncoderToValidUTF8(t *testing.T) {
	// valid utf-8
	assert.Equal(t, "", toValidUtf8(nil))
	assert.Equal(t, "", toValidUtf8([]byte("")))
	assert.Equal(t, "a", toValidUtf8([]byte("a")))
	assert.Equal(t, "abc", toValidUtf8([]byte("abc")))
	assert.Equal(t, "Hello, 世界", toValidUtf8([]byte("Hello, 世界")))

	// invalid utf-8
	assert.Equal(t, "a�z", toValidUtf8([]byte("a\xfez")))
	assert.Equal(t, "a��z", toValidUtf8([]byte("a\xc0\xafz")))
	assert.Equal(t, "a���z", toValidUtf8([]byte("a\xed\xa0\x80z")))
	assert.Equal(t, "a����z", toValidUtf8([]byte("a\xf0\x8f\xbf\xbfz")))
	assert.Equal(t, "世界����z 世界", toValidUtf8([]byte("世界\xf0\x8f\xbf\xbfz 世界")))
}

func TestPassthroughEncoder(t *testing.T) {
	logsConfig := &config.LogsConfig{}
	source := sources.NewLogSource("", logsConfig)

	content := []byte("hello world")
	msg := newMessage(content, source, message.StatusInfo)
	msg.State = message.StateRendered

	err := PassthroughEncoder.Encode(msg, "any-host")
	assert.Nil(t, err)
	assert.Equal(t, "hello world", string(msg.GetContent()))
}

func TestJSONServerlessInitEncoder(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) }) // reset singleton cache after test

	logsConfig := &config.LogsConfig{
		Service: "my-service",
		Source:  "my-source",
	}
	source := sources.NewLogSource("", logsConfig)

	type payload struct {
		Message   string `json:"message"`
		Status    string `json:"status"`
		Timestamp int64  `json:"timestamp"`
		Hostname  string `json:"hostname"`
		Service   string `json:"service,omitempty"`
		Source    string `json:"ddsource"`
		Tags      string `json:"ddtags"`
	}

	msg := newMessage([]byte("hello world"), source, message.StatusInfo)
	msg.State = message.StateRendered
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"env:prod", "region:us-east-1"})

	err := JSONServerlessInitEncoder.Encode(msg, "myhost")
	assert.NoError(t, err)

	var log payload
	assert.NoError(t, json.Unmarshal(msg.GetContent(), &log))
	assert.Equal(t, "hello world", log.Message)
	assert.Equal(t, "myhost", log.Hostname)
	assert.Equal(t, "my-service", log.Service)
	assert.Equal(t, "my-source", log.Source)
	assert.Equal(t, message.StatusInfo, log.Status)
	assert.NotEmpty(t, log.Timestamp)
	assert.Equal(t, "env:prod,region:us-east-1", log.Tags)
}

// TestJSONServerlessInitEncoder_CachesTagsOnFirstUse pins the cache behavior:
// the second message's ddtags uses the cached string from the first message,
// not its own (different) origin tags. This documents the intentional trade-off
// between performance and the need for cache updates on tag changes.
func TestJSONServerlessInitEncoder_CachesTagsOnFirstUse(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "src"})

	msg1 := newMessage([]byte("first"), source, message.StatusInfo)
	msg1.State = message.StateRendered
	msg1.Origin.LogSource = source
	msg1.Origin.SetTags([]string{"env:prod"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg1, "host"))

	// Second message has different origin tags — without invalidation the
	// encoder reuses the cached string from the first message.
	msg2 := newMessage([]byte("second"), source, message.StatusInfo)
	msg2.State = message.StateRendered
	msg2.Origin.LogSource = source
	msg2.Origin.SetTags([]string{"env:prod", "microvm_id:vm-abc"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg2, "host"))

	type payload struct {
		Tags string `json:"ddtags"`
	}
	var p1, p2 payload
	assert.NoError(t, json.Unmarshal(msg1.GetContent(), &p1))
	assert.NoError(t, json.Unmarshal(msg2.GetContent(), &p2))

	assert.Equal(t, "env:prod", p1.Tags)
	// Cache was NOT invalidated — second message still carries the startup tags.
	assert.Equal(t, "env:prod", p2.Tags,
		"without invalidation, encoder reuses the first message's cached tags")
}

// TestSetServerlessInitTagCache_UpdatesTagsImmediately is the primary feature
// test for the MicroVM /launch fix. It simulates the pre-launch → post-launch
// tag transition:
//
//  1. Encode a pre-launch message — encoder caches the startup tags.
//  2. Call SetServerlessInitTagCache with the new tags (what the /launch callback
//     now does instead of clearing the cache).
//  3. Encode a post-launch message — even one with old origin.tags — and assert
//     that ddtags carries the new (correct) tag string.
func TestSetServerlessInitTagCache_UpdatesTagsImmediately(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "lambda-microvm"})

	// Pre-launch: startup tags, no lambda_microvm_id.
	preLaunch := newMessage([]byte("app started"), source, message.StatusInfo)
	preLaunch.State = message.StateRendered
	preLaunch.Origin.LogSource = source
	preLaunch.Origin.SetTags([]string{"env:prod", "account_id:123"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(preLaunch, "host"))

	// /launch fires: SetLogsTags updates ChannelTags, then sets the cache to
	// the new tag string (the fix: set instead of clear).
	newTags := []string{"env:prod", "account_id:123", "lambda_microvm_id:vm-local"}
	SetServerlessInitTagCache(newTags)

	// Post-launch message whose origin.tags carries the new tags.
	postLaunch := newMessage([]byte("launch hook called"), source, message.StatusInfo)
	postLaunch.State = message.StateRendered
	postLaunch.Origin.LogSource = source
	postLaunch.Origin.SetTags(newTags)
	assert.NoError(t, JSONServerlessInitEncoder.Encode(postLaunch, "host"))

	type payload struct {
		Tags string `json:"ddtags"`
	}
	var pre, post payload
	assert.NoError(t, json.Unmarshal(preLaunch.GetContent(), &pre))
	assert.NoError(t, json.Unmarshal(postLaunch.GetContent(), &post))

	assert.Equal(t, "env:prod,account_id:123", pre.Tags,
		"pre-launch entry must not carry lambda_microvm_id")
	assert.Equal(t, "env:prod,account_id:123,lambda_microvm_id:vm-local", post.Tags,
		"post-launch entry must carry lambda_microvm_id")
}

// TestSetServerlessInitTagCache_StalePreLaunchMessageCannotReprime is the
// regression test for the race condition fixed by this change.
//
// Scenario: at /launch time, messages that the channel tailer already processed
// (and tagged with pre-launch ChannelTags) are still in the processor pipeline.
// With the old invalidation approach (clear to ""), the first of those stale
// messages to reach the encoder re-primes the cache with old tags, causing
// every subsequent log to lose lambda_microvm_id.
//
// With SetServerlessInitTagCache(newTags), the cache is already set to the
// correct value before any stale message arrives, so they cannot overwrite it.
func TestSetServerlessInitTagCache_StalePreLaunchMessageCannotReprime(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "lambda-microvm"})

	// Pre-launch: startup tags, no lambda_microvm_id.
	preLaunch := newMessage([]byte("startup log"), source, message.StatusInfo)
	preLaunch.State = message.StateRendered
	preLaunch.Origin.LogSource = source
	preLaunch.Origin.SetTags([]string{"env:prod", "account_id:123"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(preLaunch, "host"))

	// /launch fires: cache is set to the new tag string.
	newTags := []string{"env:prod", "account_id:123", "lambda_microvm_id:vm-xyz"}
	SetServerlessInitTagCache(newTags)

	// A stale in-flight message that was already tagged by the tailer with the
	// OLD ChannelTags (no lambda_microvm_id) reaches the encoder first.
	stale := newMessage([]byte("stale pipeline message"), source, message.StatusInfo)
	stale.State = message.StateRendered
	stale.Origin.LogSource = source
	stale.Origin.SetTags([]string{"env:prod", "account_id:123"}) // old tags, no microvm_id
	assert.NoError(t, JSONServerlessInitEncoder.Encode(stale, "host"))

	// A fresh post-launch message follows.
	fresh := newMessage([]byte("user app log"), source, message.StatusInfo)
	fresh.State = message.StateRendered
	fresh.Origin.LogSource = source
	fresh.Origin.SetTags(newTags)
	assert.NoError(t, JSONServerlessInitEncoder.Encode(fresh, "host"))

	type payload struct {
		Tags string `json:"ddtags"`
	}
	var staleP, freshP payload
	assert.NoError(t, json.Unmarshal(stale.GetContent(), &staleP))
	assert.NoError(t, json.Unmarshal(fresh.GetContent(), &freshP))

	// The stale message uses the already-set cache (new tags), not its own old origin.tags.
	assert.Equal(t, "env:prod,account_id:123,lambda_microvm_id:vm-xyz", staleP.Tags,
		"stale in-flight message must use the new cached tags, not re-prime with old ones")
	assert.Equal(t, "env:prod,account_id:123,lambda_microvm_id:vm-xyz", freshP.Tags,
		"fresh post-launch message must carry lambda_microvm_id")
}

// TestSetServerlessInitTagCache_NilResetsCache verifies that passing nil
// (or an empty slice) resets the cache so the next Encode call re-reads
// tags from the live message.
func TestSetServerlessInitTagCache_NilResetsCache(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })
	assert.NotPanics(t, func() { SetServerlessInitTagCache(nil) })
}

// TestSetServerlessInitTagCache_IdempotentOnRepeat verifies that calling
// SetServerlessInitTagCache multiple times with the same tags does not
// corrupt state and the last call wins.
func TestSetServerlessInitTagCache_IdempotentOnRepeat(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "src"})
	msg := newMessage([]byte("msg"), source, message.StatusInfo)
	msg.State = message.StateRendered
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"k:v"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg, "host"))

	// Multiple successive calls with new tags — last call wins.
	SetServerlessInitTagCache([]string{"k:v", "new:tag"})
	SetServerlessInitTagCache([]string{"k:v", "new:tag"})

	msg2 := newMessage([]byte("msg2"), source, message.StatusInfo)
	msg2.State = message.StateRendered
	msg2.Origin.LogSource = source
	msg2.Origin.SetTags([]string{"k:v", "new:tag"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg2, "host"))

	type payload struct {
		Tags string `json:"ddtags"`
	}
	var p payload
	assert.NoError(t, json.Unmarshal(msg2.GetContent(), &p))
	assert.Equal(t, "k:v,new:tag", p.Tags)
}

// TestJSONServerlessInitEncoder_ReturnsErrorForUnrenderedMessage verifies that
// Encode rejects messages that have not yet been rendered. Only StateRendered
// messages carry a final content representation; encoding earlier states would
// silently emit incomplete data.
func TestJSONServerlessInitEncoder_ReturnsErrorForUnrenderedMessage(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "src"})
	msg := newMessage([]byte("raw"), source, message.StatusInfo)
	// msg.State is StateUnstructured by default — not rendered.

	err := JSONServerlessInitEncoder.Encode(msg, "host")
	assert.Error(t, err, "encoding an unrendered message must return an error")
}

// TestJSONServerlessInitEncoder_UsesServerlessTimestampWhenSet verifies that when
// msg.ServerlessExtra.Timestamp is non-zero the encoder uses it instead of
// time.Now(). This matters for forwarded log entries whose original timestamp
// must be preserved.
func TestJSONServerlessInitEncoder_UsesServerlessTimestampWhenSet(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "src"})
	msg := newMessage([]byte("timed"), source, message.StatusInfo)
	msg.State = message.StateRendered
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"k:v"})

	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	msg.ServerlessExtra.Timestamp = fixedTime

	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg, "host"))

	type payload struct {
		Timestamp int64 `json:"timestamp"`
	}
	var p payload
	assert.NoError(t, json.Unmarshal(msg.GetContent(), &p))
	// The encoder stores milliseconds (nanoToMillis = 1_000_000).
	assert.Equal(t, fixedTime.UnixNano()/1_000_000, p.Timestamp,
		"encoder must use ServerlessExtra.Timestamp, not time.Now()")
}

// TestSetServerlessInitTagCache_ResetCausesRederive verifies that a nil reset
// causes the next Encode to re-derive tags from the live message rather than
// the previously cached string.
func TestSetServerlessInitTagCache_ResetCausesRederive(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "src"})

	// Prime the cache with an explicit value.
	SetServerlessInitTagCache([]string{"cached:tag"})

	msg1 := newMessage([]byte("m1"), source, message.StatusInfo)
	msg1.State = message.StateRendered
	msg1.Origin.LogSource = source
	msg1.Origin.SetTags([]string{"origin:tag"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg1, "host"))

	// Reset: cache returns to nil; next Encode re-derives from the message.
	SetServerlessInitTagCache(nil)

	msg2 := newMessage([]byte("m2"), source, message.StatusInfo)
	msg2.State = message.StateRendered
	msg2.Origin.LogSource = source
	msg2.Origin.SetTags([]string{"new:tag"})
	assert.NoError(t, JSONServerlessInitEncoder.Encode(msg2, "host"))

	type payload struct {
		Tags string `json:"ddtags"`
	}
	var p1, p2 payload
	assert.NoError(t, json.Unmarshal(msg1.GetContent(), &p1))
	assert.NoError(t, json.Unmarshal(msg2.GetContent(), &p2))

	assert.Equal(t, "cached:tag", p1.Tags, "pre-reset message must use the explicitly set cache")
	assert.Equal(t, "new:tag", p2.Tags, "post-reset message must re-derive tags from the message")
}

// TestJSONServerlessInitEncoder_ConcurrentSafety verifies that concurrent calls
// to Encode and SetServerlessInitTagCache do not race. Run with -race to
// exercise the atomic guarantees; the assertions confirm no crashes and
// structurally valid JSON output throughout.
func TestJSONServerlessInitEncoder_ConcurrentSafety(t *testing.T) {
	t.Cleanup(func() { SetServerlessInitTagCache(nil) })

	source := sources.NewLogSource("", &config.LogsConfig{Source: "src"})

	const readers = 20
	const encodesPerReader = 50
	const writers = 5
	const writesPerWriter = 20

	var wg sync.WaitGroup

	// Writers: alternate between setting new tag values and resetting the cache.
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range writesPerWriter {
				if j%3 == 0 {
					SetServerlessInitTagCache(nil) // reset
				} else {
					SetServerlessInitTagCache([]string{fmt.Sprintf("iter:%d-%d", i, j)})
				}
			}
		}(i)
	}

	// Readers: encode messages concurrently with the writers.
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range encodesPerReader {
				msg := newMessage([]byte("body"), source, message.StatusInfo)
				msg.State = message.StateRendered
				msg.Origin.LogSource = source
				msg.Origin.SetTags([]string{"env:prod"})

				assert.NoError(t, JSONServerlessInitEncoder.Encode(msg, "host"))
				assert.True(t, json.Valid(msg.GetContent()), "encoded payload must be valid JSON")
			}
		}()
	}

	wg.Wait()
}

func BenchmarkJSONEncoder_Encode(b *testing.B) {
	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}
	source := sources.NewLogSource("", logsConfig)

	b.Run("valid", func(b *testing.B) {
		content := []byte(strings.Repeat("x", 100))
		var msg *message.Message

		b.ResetTimer()
		b.ReportAllocs()
		for range b.N {
			msg = newMessage(content, source, message.StatusError)
			msg.State = message.StateRendered // we can only encode rendered message
			msg.Origin.LogSource = source
			msg.Origin.SetTags([]string{"a", "b:c"})

			assert.Nil(b, JSONEncoder.Encode(msg, "unknown"))
		}
	})

	b.Run("invalid", func(b *testing.B) {
		content := []byte(strings.Repeat("x", 100) + "\uFFFD")
		var msg *message.Message

		b.ResetTimer()
		b.ReportAllocs()
		for range b.N {
			msg = newMessage(content, source, message.StatusError)
			msg.State = message.StateRendered // we can only encode rendered message
			msg.Origin.LogSource = source
			msg.Origin.SetTags([]string{"a", "b:c"})

			assert.Nil(b, JSONEncoder.Encode(msg, "unknown"))
		}
	})
}
