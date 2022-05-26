// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/metadata"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestGetTagsWithRevision(t *testing.T) {
	baseTags := []string{
		"taga:valuea",
		"tagb:valueb",
	}
	resultTags := getTagsWithRevision(baseTags, "45f45")
	assert.Equal(t, 3, len(resultTags))
	assert.Equal(t, "taga:valuea", resultTags[0])
	assert.Equal(t, "tagb:valueb", resultTags[1])
	assert.Equal(t, "containerid:45f45", resultTags[2])
}

func TestWrite(t *testing.T) {
	testContent := []byte("hello this is a log")
	logChannel := make(chan *config.ChannelMessage)
	config := &Config{
		channel: logChannel,
	}
	go Write(config, testContent)
	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, testContent, received.Content)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

func TestCreateConfig(t *testing.T) {
	metadata := &metadata.Metadata{
		ContainerID: &metadata.MetadataInfo{},
	}
	config := CreateConfig(metadata)
	assert.Equal(t, 5*time.Second, config.FlushTimeout)
	assert.Equal(t, "cloudrun", config.source)
	assert.Equal(t, "DD_CLOUDRUN_LOG_AGENT", string(config.loggerName))
	assert.Equal(t, metadata, config.Metadata)
}
