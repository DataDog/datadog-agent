// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigPrecedence(t *testing.T) {
	// values that are different from defaults to be used for testing
	differentPayloadSize := 5
	differentBookmarkFrequency := 7
	differentTagEventID := true
	differentTagSID := true
	differentEventPriority := "high"
	differentInterpretMessages := false
	differentLegacyMode := true
	differentLegacyModeV2 := true
	assert.NotEqual(t, defaultConfigPayloadSize, differentPayloadSize, "Default payload_size changed and test was not updated")
	assert.NotEqual(t, defaultConfigTagEventID, differentTagEventID, "Default tag_event_id changed and test was not updated")
	assert.NotEqual(t, defaultConfigTagSID, differentTagEventID, "Default tag_sid changed and test was not updated")
	assert.NotEqual(t, defaultConfigEventPriority, differentEventPriority, "Default event_priority changed and test was not updated")
	assert.NotEqual(t, defaultConfigInterpretMessages, differentInterpretMessages, "Default interpret_messages changed and test was not updated")
	assert.NotEqual(t, defaultConfigLegacyMode, differentLegacyMode, "Default legacy_mode changed and test was not updated")
	assert.NotEqual(t, defaultConfigLegacyModeV2, differentLegacyModeV2, "Default legacy_mode_v2 changed and test was not updated")

	//
	// Assert defaults are applied
	//
	config, err := unmarshalConfig([]byte(""), []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Query, defaultConfigQuery)
		assert.Equal(t, *config.instance.Start, defaultConfigStart)
		assert.Nil(t, config.instance.Timeout)
		assert.Equal(t, *config.instance.PayloadSize, defaultConfigPayloadSize)
		assert.Equal(t, *config.instance.BookmarkFrequency, defaultConfigPayloadSize)
		assert.Equal(t, *config.instance.TagEventID, defaultConfigTagEventID)
		assert.Equal(t, *config.instance.TagSID, defaultConfigTagSID)
		assert.Equal(t, *config.instance.EventPriority, defaultConfigEventPriority)
		assert.Equal(t, *config.instance.AuthType, defaultConfigAuthType)
		assert.Nil(t, config.instance.Server)
		assert.Nil(t, config.instance.User)
		assert.Nil(t, config.instance.Domain)
		assert.Nil(t, config.instance.Password)
		assert.Equal(t, *config.instance.InterpretMessages, defaultConfigInterpretMessages)
	}

	//
	// Assert default bookmark_frequency matches custom payload_size
	//
	instanceConfig1 := []byte(fmt.Sprintf(`
payload_size: %d
`, differentPayloadSize))
	config, err = unmarshalConfig(instanceConfig1, []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.PayloadSize, differentPayloadSize)
		assert.Equal(t, *config.instance.BookmarkFrequency, differentPayloadSize)
	}

	//
	// Assert default bookmark_frequency can be different from payload_size
	//
	instanceConfig2 := []byte(fmt.Sprintf(`
payload_size: %d
bookmark_frequency: %d
`, differentPayloadSize, differentBookmarkFrequency))
	config, err = unmarshalConfig(instanceConfig2, []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.PayloadSize, differentPayloadSize)
		assert.Equal(t, *config.instance.BookmarkFrequency, differentBookmarkFrequency)
	}

	//
	// Assert init_config overrides defaults
	//
	initConfig1 := []byte(fmt.Sprintf(`
tag_event_id: %v
tag_sid: %v
event_priority: %s
interpret_messages: %v
legacy_mode: %v
legacy_mode_v2: %v
`, differentTagEventID, differentTagSID, differentEventPriority, differentInterpretMessages,
		differentLegacyMode, differentLegacyModeV2))
	config, err = unmarshalConfig([]byte(""), initConfig1)
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.TagEventID, differentTagEventID)
		assert.Equal(t, *config.instance.TagSID, differentTagSID)
		assert.Equal(t, *config.instance.EventPriority, differentEventPriority)
		assert.Equal(t, *config.instance.InterpretMessages, differentInterpretMessages)
		assert.Equal(t, *config.instance.LegacyMode, differentLegacyMode)
		assert.Equal(t, *config.instance.LegacyModeV2, differentLegacyModeV2)
	}

	//
	// Assert instance config overrides init_config
	//
	instanceConfig3 := []byte(fmt.Sprintf(`
tag_event_id: %v
tag_sid: %v
event_priority: %v
interpret_messages: %v
legacy_mode: %v
legacy_mode_v2: %v
`, defaultConfigTagEventID, defaultConfigTagSID, defaultConfigEventPriority, defaultConfigInterpretMessages,
		defaultConfigLegacyMode, defaultConfigLegacyModeV2))
	config, err = unmarshalConfig(instanceConfig3, initConfig1)
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.TagEventID, defaultConfigTagEventID)
		assert.Equal(t, *config.instance.TagSID, defaultConfigTagSID)
		assert.Equal(t, *config.instance.EventPriority, defaultConfigEventPriority)
		assert.Equal(t, *config.instance.InterpretMessages, defaultConfigInterpretMessages)
		assert.Equal(t, *config.instance.LegacyMode, defaultConfigLegacyMode)
		assert.Equal(t, *config.instance.LegacyModeV2, defaultConfigLegacyModeV2)
	}

	//
	// Assert filter sets query when query isn't provided
	//
	instanceConfig4 := []byte(`
filters:
  id:
  - 1000
`)
	config, err = unmarshalConfig(instanceConfig4, []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Query, "*[System[EventID=1000]]")
	}

	//
	// Assert query overrides filter
	//
	instanceConfig5 := []byte(`
query: banana
filters:
  id:
  - 1000
`)
	config, err = unmarshalConfig(instanceConfig5, []byte(""))
	if assert.NoError(t, err) {
		assert.Equal(t, *config.instance.Query, "banana")
	}
}
