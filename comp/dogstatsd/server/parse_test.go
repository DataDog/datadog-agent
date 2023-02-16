// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentifyEvent(t *testing.T) {
	metricSample := []byte("_e{4,5}:title|text|#shell,bash")
	messageType := findMessageType(metricSample)
	assert.Equal(t, eventType, messageType)
}

func TestIdentifyServiceCheck(t *testing.T) {
	metricSample := []byte("_sc|NAME|STATUS|d:TIMESTAMP|h:HOSTNAME|#TAG_KEY_1:TAG_VALUE_1,TAG_2|m:SERVICE_CHECK_MESSAGE")
	messageType := findMessageType(metricSample)
	assert.Equal(t, serviceCheckType, messageType)
}

func TestIdentifyMetricSample(t *testing.T) {
	metricSample := []byte("song.length:240|h|@0.5")
	messageType := findMessageType(metricSample)
	assert.Equal(t, metricSampleType, messageType)
}

func TestIdentifyRandomString(t *testing.T) {
	metricSample := []byte("song.length:240|h|@0.5")
	messageType := findMessageType(metricSample)
	assert.Equal(t, metricSampleType, messageType)
}

func TestParseTags(t *testing.T) {
	p := newParser(newFloat64ListPool())
	rawTags := []byte("tag:test,mytag,good:boy")
	tags := p.parseTags(rawTags)
	expectedTags := []string{"tag:test", "mytag", "good:boy"}
	assert.ElementsMatch(t, expectedTags, tags)
}

func TestParseTagsEmpty(t *testing.T) {
	p := newParser(newFloat64ListPool())
	rawTags := []byte("")
	tags := p.parseTags(rawTags)
	assert.Nil(t, tags)
}

func TestUnsafeParseFloat(t *testing.T) {
	rawFloat := "1.1234"

	unsafeFloat, err := parseFloat64([]byte(rawFloat))
	assert.NoError(t, err)
	float, err := strconv.ParseFloat(rawFloat, 64)
	assert.NoError(t, err)

	assert.Equal(t, float, unsafeFloat)
}

func TestUnsafeParseFloatList(t *testing.T) {
	p := newParser(newFloat64ListPool())
	unsafeFloats, err := p.parseFloat64List([]byte("1.1234:21.5:13"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 3)
	assert.Equal(t, []float64{1.1234, 21.5, 13}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 1)
	assert.Equal(t, []float64{1.1234}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234:41:"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234::41"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte(":1.1234::41"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	_, err = p.parseFloat64List([]byte(""))
	assert.Error(t, err)
}

func TestUnsafeParseInt(t *testing.T) {
	rawInt := "123"

	unsafeInteger, err := parseInt64([]byte(rawInt))
	assert.NoError(t, err)
	integer, err := strconv.ParseInt(rawInt, 10, 64)
	assert.NoError(t, err)

	assert.Equal(t, integer, unsafeInteger)
}
