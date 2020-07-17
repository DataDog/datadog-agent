package dogstatsd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseEvent(rawEvent []byte) (dogstatsdEvent, error) {
	parser := newParser()
	return parser.parseEvent(rawEvent)
}

func TestEventMinimal(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMultilinesText(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,24}:test title|test\\line1\\nline2\\nline3"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test\\line1\nline2\nline3"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventPipeInTitle(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"))

	require.Nil(t, err)
	assert.Equal(t, string("test|title"), e.title)
	assert.Equal(t, string("test\\line1\nline2\nline3"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventError(t *testing.T) {
	// missing length header
	_, err := parseEvent([]byte("_e:title|text"))
	assert.Error(t, err)

	// greater length than packet
	_, err = parseEvent([]byte("_e{10,10}:title|text"))
	assert.Error(t, err)

	// zero length
	_, err = parseEvent([]byte("_e{0,0}:a|a"))
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEvent([]byte("_e{5555:title|text"))
	assert.Error(t, err)

	// missing wrong len format
	_, err = parseEvent([]byte("_e{a,1}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent([]byte("_e{1,a}:title|text"))
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEvent([]byte("_e{5,}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent([]byte("_e{,4}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent([]byte("_e{}:title|text"))
	assert.Error(t, err)

	_, err = parseEvent([]byte("_e{,}:title|text"))
	assert.Error(t, err)

	// not enough information
	_, err = parseEvent([]byte("_e|text"))
	assert.Error(t, err)

	_, err = parseEvent([]byte("_e:|text"))
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseEvent([]byte("_e{5,4}:title|text|d:abc"))
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEvent([]byte("_e{5,4}:title|text|p:urgent"))
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEvent([]byte("_e{5,4}:title|text|p:urgent"))
	assert.NoError(t, err)

	// invalid alert type
	_, err = parseEvent([]byte("_e{5,4}:title|text|t:test"))
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseEvent([]byte("_e{5,4}:title|text|x:1234"))
	assert.NoError(t, err)
}

func TestEventMetadataTimestamp(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|d:21"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(21), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMetadataPriority(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|p:low"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityLow, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMetadataHostname(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|h:localhost"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMetadataAlertType(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|t:warning"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeWarning, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMetadataAggregatioKey(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|k:some aggregation key"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, string("some aggregation key"), e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMetadataSourceType(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|s:this is the source"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string(nil), e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, string("this is the source"), e.sourceType)
}

func TestEventMetadataTags(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|#tag1,tag2:test"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(0), e.timestamp)
	assert.Equal(t, priorityNormal, e.priority)
	assert.Equal(t, []string{string("tag1"), string("tag2:test")}, e.tags)
	assert.Equal(t, alertTypeInfo, e.alertType)
	assert.Equal(t, "", e.aggregationKey)
	assert.Equal(t, "", e.sourceType)
}

func TestEventMetadataMultiple(t *testing.T) {
	e, err := parseEvent([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))

	require.Nil(t, err)
	assert.Equal(t, string("test title"), e.title)
	assert.Equal(t, string("test text"), e.text)
	assert.Equal(t, int64(12345), e.timestamp)
	assert.Equal(t, priorityLow, e.priority)
	assert.Equal(t, []string{string("tag1"), string("tag2:test")}, e.tags)
	assert.Equal(t, alertTypeWarning, e.alertType)
	assert.Equal(t, string("aggKey"), e.aggregationKey)
	assert.Equal(t, string("source test"), e.sourceType)
}
