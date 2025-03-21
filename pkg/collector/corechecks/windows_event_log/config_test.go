// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/stretchr/testify/assert"
)

func assertOptionalValue[T any](t *testing.T, assertCompare assert.ComparisonAssertionFunc, o optional.Option[T], expected T) bool {
	actual, isSet := o.Get()
	return assert.True(t, isSet, fmt.Sprintf("%v is not set", o)) &&
		assertCompare(t, expected, actual, fmt.Sprintf("%v does not match expcted value", o))
}

func TestConfigPrecedence(t *testing.T) {
	// values that are different from defaults to be used for testing
	differentStart := "oldest"
	differentPayloadSize := 5
	differentBookmarkFrequency := 7
	differentTagEventID := true
	differentTagSID := true
	differentEventPriority := "high"
	differentAuthType := "kerberos"
	differentInterpretMessages := false
	differentLegacyMode := true
	differentLegacyModeV2 := true
	assert.NotEqual(t, defaultConfigStart, differentStart, "Default start changed and test was not updated")
	assert.NotEqual(t, defaultConfigPayloadSize, differentPayloadSize, "Default payload_size changed and test was not updated")
	assert.NotEqual(t, defaultConfigTagEventID, differentTagEventID, "Default tag_event_id changed and test was not updated")
	assert.NotEqual(t, defaultConfigTagSID, differentTagEventID, "Default tag_sid changed and test was not updated")
	assert.NotEqual(t, defaultConfigEventPriority, differentEventPriority, "Default event_priority changed and test was not updated")
	assert.NotEqual(t, defaultConfigAuthType, differentAuthType, "Default auth_type changed and test was not updated")
	assert.NotEqual(t, defaultConfigInterpretMessages, differentInterpretMessages, "Default interpret_messages changed and test was not updated")
	assert.NotEqual(t, defaultConfigLegacyMode, differentLegacyMode, "Default legacy_mode changed and test was not updated")
	assert.NotEqual(t, defaultConfigLegacyModeV2, differentLegacyModeV2, "Default legacy_mode_v2 changed and test was not updated")

	//
	// Assert defaults are applied
	//
	config, err := unmarshalConfig([]byte(""), []byte(""))
	if assert.NoError(t, err) {
		assertOptionalValue(t, assert.Equal, config.instance.Query, defaultConfigQuery)
		assertOptionalValue(t, assert.Equal, config.instance.Start, defaultConfigStart)
		_, isSet := config.instance.Timeout.Get()
		assert.False(t, isSet)
		assertOptionalValue(t, assert.Equal, config.instance.PayloadSize, defaultConfigPayloadSize)
		assertOptionalValue(t, assert.Equal, config.instance.BookmarkFrequency, defaultConfigPayloadSize)
		assertOptionalValue(t, assert.Equal, config.instance.TagEventID, defaultConfigTagEventID)
		assertOptionalValue(t, assert.Equal, config.instance.TagSID, defaultConfigTagSID)
		assertOptionalValue(t, assert.Equal, config.instance.EventPriority, defaultConfigEventPriority)
		assertOptionalValue(t, assert.Equal, config.instance.AuthType, defaultConfigAuthType)
		_, isSet = config.instance.Server.Get()
		assert.False(t, isSet)
		_, isSet = config.instance.User.Get()
		assert.False(t, isSet)
		_, isSet = config.instance.Domain.Get()
		assert.False(t, isSet)
		_, isSet = config.instance.Password.Get()
		assert.False(t, isSet)
		assertOptionalValue(t, assert.Equal, config.instance.InterpretMessages, defaultConfigInterpretMessages)
	}

	//
	// Assert default bookmark_frequency matches custom payload_size
	//
	instanceConfig1 := []byte(fmt.Sprintf(`
payload_size: %d
`, differentPayloadSize))
	config, err = unmarshalConfig(instanceConfig1, []byte(""))
	if assert.NoError(t, err) {
		assertOptionalValue(t, assert.Equal, config.instance.PayloadSize, differentPayloadSize)
		assertOptionalValue(t, assert.Equal, config.instance.BookmarkFrequency, differentPayloadSize)
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
		assertOptionalValue(t, assert.Equal, config.instance.PayloadSize, differentPayloadSize)
		assertOptionalValue(t, assert.Equal, config.instance.BookmarkFrequency, differentBookmarkFrequency)
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
		assertOptionalValue(t, assert.Equal, config.instance.TagEventID, differentTagEventID)
		assertOptionalValue(t, assert.Equal, config.instance.TagSID, differentTagSID)
		assertOptionalValue(t, assert.Equal, config.instance.EventPriority, differentEventPriority)
		assertOptionalValue(t, assert.Equal, config.instance.InterpretMessages, differentInterpretMessages)
		assertOptionalValue(t, assert.Equal, config.instance.LegacyMode, differentLegacyMode)
		assertOptionalValue(t, assert.Equal, config.instance.LegacyModeV2, differentLegacyModeV2)
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
		assertOptionalValue(t, assert.Equal, config.instance.TagEventID, defaultConfigTagEventID)
		assertOptionalValue(t, assert.Equal, config.instance.TagSID, defaultConfigTagSID)
		assertOptionalValue(t, assert.Equal, config.instance.EventPriority, defaultConfigEventPriority)
		assertOptionalValue(t, assert.Equal, config.instance.InterpretMessages, defaultConfigInterpretMessages)
		assertOptionalValue(t, assert.Equal, config.instance.LegacyMode, defaultConfigLegacyMode)
		assertOptionalValue(t, assert.Equal, config.instance.LegacyModeV2, defaultConfigLegacyModeV2)
	}

	//
	// Assert instance config overrides defaults (for opts without init_config)
	//
	instanceConfig6 := []byte(fmt.Sprintf(`
start: %v
auth_type: %v
payload_size: %v
bookmark_frequency: %v
`, differentStart, differentAuthType, differentPayloadSize, differentBookmarkFrequency))
	config, err = unmarshalConfig(instanceConfig6, nil)
	if assert.NoError(t, err) {
		assertOptionalValue(t, assert.Equal, config.instance.Start, differentStart)
		assertOptionalValue(t, assert.Equal, config.instance.AuthType, differentAuthType)
		assertOptionalValue(t, assert.Equal, config.instance.PayloadSize, differentPayloadSize)
		assertOptionalValue(t, assert.Equal, config.instance.BookmarkFrequency, differentBookmarkFrequency)
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
		assertOptionalValue(t, assert.Equal, config.instance.Query, "*[System[EventID=1000]]")
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
		assertOptionalValue(t, assert.Equal, config.instance.Query, "banana")
	}
}

// TestInvalidRegexp tests that we get an error for regexp patterns we expect are not supported
// https://github.com/google/re2/wiki/Syntax
func TestInvalidRegexp(t *testing.T) {
	tcs := []struct {
		name       string
		pattern    string
		errorMatch string
	}{
		{"lookahead", "(?=foo)", "invalid or unsupported Perl syntax: `(?=`"},
		{"lookbehind", "(?<=foo)", "invalid or unsupported Perl syntax: `(?<`"},
		{"negative lookahead", "(?!foo)", "invalid or unsupported Perl syntax: `(?!`"},
		{"negative lookbehind", "(?<!foo)", "invalid or unsupported Perl syntax: `(?<`"},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compileRegexPatterns([]string{tc.pattern})
			assert.ErrorContains(t, err, tc.errorMatch)
		})
	}
}
