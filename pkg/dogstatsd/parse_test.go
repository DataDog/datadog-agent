package dogstatsd

import (
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
	rawTags := []byte("tag:test,mytag,good:boy")
	tags := parseTags(rawTags)
	expectedTags := [][]byte{[]byte("tag:test"), []byte("mytag"), []byte("good:boy")}
	assert.ElementsMatch(t, expectedTags, tags)
}

func TestParseTagsEmpty(t *testing.T) {
	rawTags := []byte("")
	tags := parseTags(rawTags)
	assert.Nil(t, tags)
}
