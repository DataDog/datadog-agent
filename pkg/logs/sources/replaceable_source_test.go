// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestNewReplaceableSource(t *testing.T) {
	source := NewLogSource("test", &config.LogsConfig{Type: "file"})
	replaceable := NewReplaceableSource(source)

	require.NotNil(t, replaceable)
	assert.Equal(t, source, replaceable.UnderlyingSource())
}

func TestReplaceableSourceReplace(t *testing.T) {
	source1 := NewLogSource("source1", &config.LogsConfig{Type: "file"})
	source2 := NewLogSource("source2", &config.LogsConfig{Type: "tcp"})

	replaceable := NewReplaceableSource(source1)
	assert.Equal(t, source1, replaceable.UnderlyingSource())

	replaceable.Replace(source2)
	assert.Equal(t, source2, replaceable.UnderlyingSource())
}

func TestReplaceableSourceStatus(t *testing.T) {
	source := NewLogSource("test", &config.LogsConfig{Type: "file"})
	replaceable := NewReplaceableSource(source)

	status := replaceable.Status()
	assert.NotNil(t, status)
	assert.Equal(t, source.Status, status)
}

func TestReplaceableSourceConfig(t *testing.T) {
	cfg := &config.LogsConfig{Type: "file", Path: "/var/log/test.log"}
	source := NewLogSource("test", cfg)
	replaceable := NewReplaceableSource(source)

	config := replaceable.Config()
	assert.NotNil(t, config)
	assert.Equal(t, cfg, config)
	assert.Equal(t, "file", config.Type)
	assert.Equal(t, "/var/log/test.log", config.Path)
}

func TestReplaceableSourceAddRemoveInput(t *testing.T) {
	source := NewLogSource("test", nil)
	replaceable := NewReplaceableSource(source)

	// Add input
	replaceable.AddInput("input1")
	inputs := source.GetInputs()
	assert.Contains(t, inputs, "input1")

	// Remove input
	replaceable.RemoveInput("input1")
	inputs = source.GetInputs()
	assert.NotContains(t, inputs, "input1")
}

func TestReplaceableSourceRecordBytes(t *testing.T) {
	source := NewLogSource("test", nil)
	replaceable := NewReplaceableSource(source)

	replaceable.RecordBytes(100)
	assert.Equal(t, int64(100), source.BytesRead.Get())

	replaceable.RecordBytes(50)
	assert.Equal(t, int64(150), source.BytesRead.Get())
}

func TestReplaceableSourceGetSourceType(t *testing.T) {
	source := NewLogSource("test", nil)
	source.SetSourceType(DockerSourceType)

	replaceable := NewReplaceableSource(source)

	sourceType := replaceable.GetSourceType()
	assert.Equal(t, DockerSourceType, sourceType)
}

func TestReplaceableSourceRegisterAndGetInfo(t *testing.T) {
	source := NewLogSource("test", nil)
	replaceable := NewReplaceableSource(source)

	// Get info should return the registered BytesRead provider
	info := replaceable.GetInfo("Bytes Read")
	assert.NotNil(t, info)

	// Non-existent key should return nil
	info = replaceable.GetInfo("nonexistent")
	assert.Nil(t, info)
}

func TestReplaceableSourceConcurrentAccess(t *testing.T) {
	source1 := NewLogSource("source1", &config.LogsConfig{Type: "file"})
	source2 := NewLogSource("source2", &config.LogsConfig{Type: "tcp"})
	replaceable := NewReplaceableSource(source1)

	done := make(chan bool)

	// Concurrent reads and writes
	go func() {
		for i := 0; i < 100; i++ {
			_ = replaceable.Status()
			_ = replaceable.Config()
			_ = replaceable.UnderlyingSource()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				replaceable.Replace(source1)
			} else {
				replaceable.Replace(source2)
			}
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			replaceable.AddInput("input")
			replaceable.RemoveInput("input")
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}
