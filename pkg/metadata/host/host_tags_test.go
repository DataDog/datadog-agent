package host

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestGetHostTags(t *testing.T) {
	mockConfig := config.NewMock()
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3"})
	defer mockConfig.Set("tags", nil)

	hostTags := getHostTags()
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetEmptyHostTags(t *testing.T) {
	// getHostTags should never return a nil value under System even when there are no host tags
	hostTags := getHostTags()
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{}, hostTags.System)
}

func TestGetHostTagsWithSplits(t *testing.T) {
	mockConfig := config.NewMock()
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})
	defer mockConfig.Set("tags", nil)

	hostTags := getHostTags()
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0", "kafka_partition:1", "kafka_partition:2"}, hostTags.System)
}

func TestGetHostTagsWithoutSplits(t *testing.T) {
	mockConfig := config.NewMock()
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})
	defer mockConfig.Set("tags", nil)

	hostTags := getHostTags()
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"}, hostTags.System)
}
