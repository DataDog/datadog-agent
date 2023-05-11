// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtlog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigPrecedence(t *testing.T) {
	// values that are different from defaults to be used for testing
	differentPayload_size := 5
	differentBookmark_frequency := 7
	differentTag_event_id := true
	differentTag_sid := true
	differentEvent_priority := "high"
	assert.NotEqual(t, defaultConfigPayload_size, differentPayload_size, "Default payload_size changed and test was not updated")
	assert.NotEqual(t, defaultConfigTag_event_id, differentTag_event_id, "Default tag_event_id changed and test was not updated")
	assert.NotEqual(t, defaultConfigTag_sid, differentTag_event_id, "Default tag_sid changed and test was not updated")
	assert.NotEqual(t, defaultConfigEvent_priority, differentEvent_priority, "Default event_priority changed and test was not updated")

	//
	// Assert defaults are applied
	//
	config, err := UnmarshalConfig([]byte(""), []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Query, defaultConfigQuery)
		assert.Equal(t, *config.instance.Start, defaultConfigStart)
		assert.Equal(t, *config.instance.Timeout, defaultConfigTimeout)
		assert.Equal(t, *config.instance.Payload_size, defaultConfigPayload_size)
		assert.Equal(t, *config.instance.Bookmark_frequency, defaultConfigPayload_size)
		assert.Equal(t, *config.instance.Tag_event_id, defaultConfigTag_event_id)
		assert.Equal(t, *config.instance.Tag_sid, defaultConfigTag_sid)
		assert.Equal(t, *config.instance.Event_priority, defaultConfigEvent_priority)
	}

	//
	// Assert default bookmark_frequency matches custom payload_size
	//
	instanceConfig1 := []byte(fmt.Sprintf(`
payload_size: %d
`, differentPayload_size))
	config, err = UnmarshalConfig(instanceConfig1, []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Payload_size, differentPayload_size)
		assert.Equal(t, *config.instance.Bookmark_frequency, differentPayload_size)
	}

	//
	// Assert default bookmark_frequency can be different from payload_size
	//
	instanceConfig2 := []byte(fmt.Sprintf(`
payload_size: %d
bookmark_frequency: %d
`, differentPayload_size, differentBookmark_frequency))
	config, err = UnmarshalConfig(instanceConfig2, []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Payload_size, differentPayload_size)
		assert.Equal(t, *config.instance.Bookmark_frequency, differentBookmark_frequency)
	}

	//
	// Assert init_config overrides defaults
	//
	initConfig1 := []byte(fmt.Sprintf(`
tag_event_id: %v
tag_sid: %v
event_priority: %s
`, differentTag_event_id, differentTag_sid, differentEvent_priority))
	config, err = UnmarshalConfig([]byte(""), initConfig1)
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Tag_event_id, differentTag_event_id)
		assert.Equal(t, *config.instance.Tag_sid, differentTag_sid)
		assert.Equal(t, *config.instance.Event_priority, differentEvent_priority)
	}

	//
	// Assert instance config overrides init_config
	//
	instanceConfig3 := []byte(fmt.Sprintf(`
tag_event_id: %v
tag_sid: %v
event_priority: %v
`, defaultConfigTag_event_id, defaultConfigTag_sid, defaultConfigEvent_priority))
	config, err = UnmarshalConfig(instanceConfig3, initConfig1)
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Tag_event_id, defaultConfigTag_event_id)
		assert.Equal(t, *config.instance.Tag_sid, defaultConfigTag_sid)
		assert.Equal(t, *config.instance.Event_priority, defaultConfigEvent_priority)
	}
}
