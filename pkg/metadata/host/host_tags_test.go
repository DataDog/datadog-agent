package host

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func init() {
	retrySleepTime = 0
}

func TestGetHostTags(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3"})
	defer mockConfig.Set("tags", nil)

	hostTags := GetHostTags(false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetEmptyHostTags(t *testing.T) {
	// getHostTags should never return a nil value under System even when there are no host tags
	hostTags := GetHostTags(false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{}, hostTags.System)
}

func TestGetHostTagsWithSplits(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})
	defer mockConfig.Set("tags", nil)

	hostTags := GetHostTags(false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0", "kafka_partition:1", "kafka_partition:2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetHostTagsWithoutSplits(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})
	defer mockConfig.Set("tags", nil)

	hostTags := GetHostTags(false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0,1,2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetHostTagsWithEnv(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "env:prod"})
	mockConfig.Set("env", "preprod")
	defer mockConfig.Set("tags", nil)
	defer mockConfig.Set("env", "")

	hostTags := GetHostTags(false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"env:preprod", "env:prod", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestMarshalEmptyHostTags(t *testing.T) {
	tags := &Tags{
		System:              []string{},
		GoogleCloudPlatform: []string{},
	}

	marshaled, _ := json.Marshal(tags)

	// `System` should be marshaled as an empty list
	assert.Equal(t, string(marshaled), `{"system":[]}`)
}

func TestCombineExtraTags(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag4"})
	mockConfig.Set("extra_tags", []string{"tag1:value2", "tag3", "tag4"})
	defer mockConfig.Set("tags", nil)
	defer mockConfig.Set("extra_tags", nil)

	hostTags := GetHostTags(false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag1:value2", "tag2", "tag3", "tag4"}, hostTags.System)
}
