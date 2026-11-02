// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseEvent(t *testing.T, rawEvent []byte) (Event, error) {
	parser := testParser(t, testOptions())
	return parser.ParseEvent(rawEvent)
}

func TestEventMinimal(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMultilinesText(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,24}:test title|test\\line1\\nline2\\nline3"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test\\line1\nline2\nline3"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventPipeInTitle(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"))

	require.Nil(t, err)
	assert.Equal(t, string("test|title"), e.Title)
	assert.Equal(t, string("test\\line1\nline2\nline3"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventError(t *testing.T) {
	// missing length header
	_, err := parseEvent(t, []byte("_e:title|text"))
	assert.Error(t, err)

	// greater length than packet
	_, err = parseEvent(t, []byte("_e{10,10}:title|text"))
	assert.Error(t, err)

	// zero length title
	_, err = parseEvent(t, []byte("_e{0,0}:a|a"))
	assert.Error(t, err)

	_, err = parseEvent(t, []byte("_e{0,4}:text"))
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEvent(t, []byte("_e{5555:title|text"))
	assert.Error(t, err)

	// missing wrong len format
	_, err = parseEvent(t, []byte("_e{a,1}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent(t, []byte("_e{1,a}:title|text"))
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEvent(t, []byte("_e{5,}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent(t, []byte("_e{,4}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent(t, []byte("_e{}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent(t, []byte("_e{,}:title|text"))
	assert.Error(t, err)

	// not enough information
	_, err = parseEvent(t, []byte("_e|text"))
	assert.Error(t, err)

	_, err = parseEvent(t, []byte("_e:|text"))
	assert.Error(t, err)

	// invalid title length
	_, err = parseEvent(t, []byte("_e{-123,-987}:"))
	assert.Error(t, err)

	// invalid text length
	_, err = parseEvent(t, []byte("_e{5,-987}:title"))
	assert.Error(t, err)

	// malformed message
	_, err = parseEvent(t, []byte("_e{0001,-9876"))
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseEvent(t, []byte("_e{5,4}:title|text|d:abc"))
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEvent(t, []byte("_e{5,4}:title|text|p:urgent"))
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEvent(t, []byte("_e{5,4}:title|text|p:urgent"))
	assert.NoError(t, err)

	// invalid alert type
	_, err = parseEvent(t, []byte("_e{5,4}:title|text|t:test"))
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseEvent(t, []byte("_e{5,4}:title|text|x:1234"))
	assert.NoError(t, err)
}

func TestEventMetadataTimestamp(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|d:21"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(21), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMetadataPriority(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|p:low"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityLow, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMetadataHostname(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|h:localhost"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMetadataAlertType(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|t:warning"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertWarning, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMetadataAggregatioKey(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|k:some aggregation key"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, string("some aggregation key"), e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMetadataSourceType(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|s:this is the source"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, string("this is the source"), e.SourceType)
}

func TestEventMetadataTags(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|#tag1,tag2:test"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(0), e.Timestamp)
	assert.Equal(t, PriorityNormal, e.Priority)
	assert.Equal(t, []string{string("tag1"), string("tag2:test")}, e.Tags)
	assert.Equal(t, AlertInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceType)
}

func TestEventMetadataMultiple(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string("test text"), e.Text)
	assert.Equal(t, int64(12345), e.Timestamp)
	assert.Equal(t, PriorityLow, e.Priority)
	assert.Equal(t, []string{string("tag1"), string("tag2:test")}, e.Tags)
	assert.Equal(t, AlertWarning, e.AlertType)
	assert.Equal(t, string("aggKey"), e.AggregationKey)
	assert.Equal(t, string("source test"), e.SourceType)
}

func TestEventEmptyTitle(t *testing.T) {
	_, err := parseEvent(t, []byte("_e{0,9}:|test text"))

	require.Error(t, err, "invalid event: empty title")
}

func TestEventEmptyText(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,0}:test title|"))

	require.NoError(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string(""), e.Text)
}

func TestEventEmptyTextWithAlertType(t *testing.T) {
	e, err := parseEvent(t, []byte("_e{10,0}:test title||t:warning"))

	require.NoError(t, err)
	assert.Equal(t, string("test title"), e.Title)
	assert.Equal(t, string(""), e.Text)
	assert.Equal(t, AlertWarning, e.AlertType)
}
