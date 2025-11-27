// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tracermetadata

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSkipServiceTag(t *testing.T) {
	ustService := "my-service"
	ustEnv := "my-env"
	ustVersion := "my-version"

	tests := []struct {
		tagKey       string
		tagValue     string
		expectedSkip bool
	}{
		{
			tagKey:       "tracer_service_name",
			tagValue:     "my-service",
			expectedSkip: true,
		},
		{
			tagKey:       "tracer_service_name",
			tagValue:     "not-my-service",
			expectedSkip: false,
		},
		{
			tagKey:       "tracer_service_env",
			tagValue:     "my-env",
			expectedSkip: true,
		},
		{
			tagKey:       "tracer_service_version",
			tagValue:     "my-version",
			expectedSkip: true,
		},
		{
			tagKey:       "tracer_service_version",
			tagValue:     "not-my-version",
			expectedSkip: false,
		},
	}
	for _, tt := range tests {
		skip := ShouldSkipServiceTagKV(tt.tagKey, tt.tagValue, ustService, ustEnv, ustVersion)
		assert.Equal(t, tt.expectedSkip, skip)

		skip = ShouldSkipServiceTag(tt.tagKey+":"+tt.tagValue, ustService, ustEnv, ustVersion)
		assert.Equal(t, tt.expectedSkip, skip)
	}
}

func TestTags(t *testing.T) {
	trm := TracerMetadata{}
	tags := trm.GetTags()
	assert.Empty(t, tags)

	trm = TracerMetadata{
		ProcessTags: "entrypoint.name:com.example.App,service.type:tomcat,service.framework:spring",
	}
	tags = trm.GetTags()
	assert.Equal(t, []string{
		"entrypoint.name:com.example.App",
		"service.type:tomcat",
		"service.framework:spring",
	}, tags)

	trm = TracerMetadata{
		ServiceName:    "my-service",
		ServiceEnv:     "my-env",
		ServiceVersion: "my-version",
		ProcessTags:    "entrypoint.name:com.example.App,service.type:tomcat,service.framework:spring",
	}
	tags = trm.GetTags()
	assert.Equal(t, []string{
		"tracer_service_name:my-service",
		"tracer_service_env:my-env",
		"tracer_service_version:my-version",
		"entrypoint.name:com.example.App",
		"service.type:tomcat",
		"service.framework:spring",
	}, tags)
	i := 0
	for key, value := range trm.Tags() {
		assert.Equal(t, tags[i], key+":"+value)
		i++
	}
}

func TestParseProcessTags(t *testing.T) {
	t.Helper()
	tests := []struct {
		name         string
		processTags  string
		expectedTags []string
	}{
		{
			name:        "valid comma-separated tags",
			processTags: "entrypoint.name:com.example.App,service.type:tomcat,service.framework:spring",
			expectedTags: []string{
				"entrypoint.name:com.example.App",
				"service.framework:spring",
				"service.type:tomcat",
			},
		},
		{
			name:        "single tag",
			processTags: "entrypoint.workdir:app",
			expectedTags: []string{
				"entrypoint.workdir:app",
			},
		},
		{
			name:         "empty string",
			processTags:  "",
			expectedTags: nil,
		},
		{
			name:        "tags with spaces",
			processTags: " service.runtime : java-17 , entrypoint.name : com.app.Main ",
			expectedTags: []string{
				"entrypoint.name:com.app.Main",
				"service.runtime:java-17",
			},
		},
		{
			name:        "tag with colon in value",
			processTags: "url:http://example.com:8080",
			expectedTags: []string{
				"url:http://example.com:8080",
			},
		},
		{
			name:         "malformed tag without colon",
			processTags:  "invalid_tag,service.type:nginx",
			expectedTags: []string{"service.type:nginx"},
		},
		{
			name:         "tags with empty keys or values",
			processTags:  ":value,key:,service.runtime:python3.9",
			expectedTags: []string{"service.runtime:python3.9"},
		},
		{
			name:         "empty tag entries",
			processTags:  "service.framework:django,,entrypoint.name:manage.py,",
			expectedTags: []string{"entrypoint.name:manage.py", "service.framework:django"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trm := TracerMetadata{
				ProcessTags: tt.processTags,
			}

			tags := trm.GetTags()
			sort.Strings(tags)
			sort.Strings(tt.expectedTags)
			assert.Equal(t, tt.expectedTags, tags)
		})
	}
}
