// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedRegistryLoads(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.NotEmpty(t, r.Version())
}

func TestGetAttributePrecedence_KnownConcept(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	tests := []struct {
		name     string
		concept  Concept
		expected []TagInfo
	}{
		{
			name:    "db.query chain",
			concept: ConceptDBQuery,
			expected: []TagInfo{
				{Name: "db.query.text", Provider: ProviderOTel, Version: "1.24.0", Type: ValueTypeString},
				{Name: "db.statement", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "sql.query", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "mongodb.query", Provider: ProviderDatadog, Type: ValueTypeString},
			},
		},
		{
			name:    "http.status_code chain",
			concept: ConceptHTTPStatusCode,
			expected: []TagInfo{
				{Name: "http.status_code", Provider: ProviderDatadog, Type: ValueTypeInt64},
				{Name: "http.status_code", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "http.response.status_code", Provider: ProviderOTel, Version: "1.21.0", Type: ValueTypeString},
			},
		},
		{
			name:    "peer.hostname chain",
			concept: ConceptPeerHostname,
			expected: []TagInfo{
				{Name: "peer.hostname", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "hostname", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "net.peer.name", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "db.hostname", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "network.destination.name", Provider: ProviderOTel, Version: "1.21.0", Type: ValueTypeString},
				{Name: "grpc.host", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "http.host", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "server.address", Provider: ProviderOTel, Version: "1.21.0", Type: ValueTypeString},
				{Name: "http.server_name", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "out.host", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "dns.hostname", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "network.destination.ip", Provider: ProviderOTel, Version: "1.21.0", Type: ValueTypeString},
			},
		},
		{
			name:    "rpc.grpc.status_code chain",
			concept: ConceptGRPCStatusCode,
			expected: []TagInfo{
				{Name: "rpc.grpc.status_code", Provider: ProviderOTel, Type: ValueTypeInt64},
				{Name: "grpc.code", Provider: ProviderDatadog, Type: ValueTypeInt64},
				{Name: "rpc.grpc.status.code", Provider: ProviderOTel, Type: ValueTypeInt64},
				{Name: "grpc.status.code", Provider: ProviderDatadog, Type: ValueTypeInt64},
				{Name: "rpc.grpc.status_code", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "grpc.code", Provider: ProviderDatadog, Type: ValueTypeString},
			},
		},
		{
			name:    "http.method chain",
			concept: ConceptHTTPMethod,
			expected: []TagInfo{
				{Name: "http.request.method", Provider: ProviderOTel, Version: "1.21.0", Type: ValueTypeString},
				{Name: "http.method", Provider: ProviderOTel, Type: ValueTypeString},
			},
		},
		{
			name:    "peer.db.name chain",
			concept: ConceptPeerDBName,
			expected: []TagInfo{
				{Name: "db.name", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "mongodb.db", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "db.instance", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "cassandra.keyspace", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "db.namespace", Provider: ProviderOTel, Version: "1.24.0", Type: ValueTypeString},
			},
		},
		{
			name:    "peer.messaging.destination chain",
			concept: ConceptPeerMessagingDestination,
			expected: []TagInfo{
				{Name: "topicname", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "messaging.destination", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "messaging.destination.name", Provider: ProviderOTel, Version: "1.21.0", Type: ValueTypeString},
				{Name: "messaging.rabbitmq.exchange", Provider: ProviderOTel, Type: ValueTypeString},
				{Name: "amqp.destination", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "amqp.queue", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "amqp.exchange", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "msmq.queue.path", Provider: ProviderDatadog, Type: ValueTypeString},
				{Name: "aws.queue.name", Provider: ProviderDatadog, Type: ValueTypeString},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := r.GetAttributePrecedence(tt.concept)
			require.NotNil(t, tags, "expected tags for concept %s", tt.concept)
			require.Len(t, tags, len(tt.expected), "expected %d tags for concept %s", len(tt.expected), tt.concept)

			for i, expected := range tt.expected {
				actual := tags[i]
				assert.Equal(t, expected.Name, actual.Name, "tag[%d].Name mismatch", i)
				assert.Equal(t, expected.Provider, actual.Provider, "tag[%d].Provider mismatch for %s", i, expected.Name)
				assert.Equal(t, expected.Version, actual.Version, "tag[%d].Version mismatch for %s", i, expected.Name)
				assert.Equal(t, expected.Type, actual.Type, "tag[%d].Type mismatch for %s", i, expected.Name)
			}
		})
	}
}

func TestGetAttributePrecedence_UnknownConcept(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	tags := r.GetAttributePrecedence(Concept("unknown.concept"))
	assert.Nil(t, tags)
}

func TestGetAttributePrecedence_SingleTag(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	// Concepts with a single tag should return that tag with explicit type
	tests := []struct {
		concept  Concept
		expected TagInfo
	}{
		{
			concept:  ConceptPeerService,
			expected: TagInfo{Name: "peer.service", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptDDMeasured,
			expected: TagInfo{Name: "_dd.measured", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptMongoDBQuery,
			expected: TagInfo{Name: "mongodb.query", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptElasticsearchBody,
			expected: TagInfo{Name: "elasticsearch.body", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptRedisRawCommand,
			expected: TagInfo{Name: "redis.raw_command", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptComponent,
			expected: TagInfo{Name: "component", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptLinkName,
			expected: TagInfo{Name: "link.name", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptDDBaseService,
			expected: TagInfo{Name: "_dd.base_service", Provider: ProviderDatadog, Version: "", Type: ValueTypeString},
		},
		{
			concept:  ConceptSamplingPriority,
			expected: TagInfo{Name: "_sampling_priority_v1", Provider: ProviderDatadog, Version: "", Type: ValueTypeFloat64},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.concept), func(t *testing.T) {
			tags := r.GetAttributePrecedence(tt.concept)
			require.NotNil(t, tags, "expected tags for concept %s", tt.concept)
			require.Len(t, tags, 1, "expected exactly 1 tag for concept %s", tt.concept)

			assert.Equal(t, tt.expected.Name, tags[0].Name)
			assert.Equal(t, tt.expected.Provider, tags[0].Provider)
			assert.Equal(t, tt.expected.Version, tags[0].Version)
			assert.Equal(t, tt.expected.Type, tags[0].Type)
		})
	}
}

func TestGetAllEquivalences(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	all := r.GetAllEquivalences()
	require.NotNil(t, all)

	// Check that we have a reasonable number of concepts
	assert.GreaterOrEqual(t, len(all), 35, "expected at least 35 concepts")

	// Verify some known concepts exist
	_, ok := all[ConceptDBQuery]
	assert.True(t, ok, "expected db.query concept")

	_, ok = all[ConceptHTTPStatusCode]
	assert.True(t, ok, "expected http.status_code concept")

	_, ok = all[ConceptPeerHostname]
	assert.True(t, ok, "expected peer.hostname concept")
}

func TestVersion(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	version := r.Version()
	assert.NotEmpty(t, version)
	assert.Equal(t, "0.1.0", version)
}

func TestTagInfoMetadata(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("db.query metadata", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptDBQuery)
		require.NotNil(t, tags)
		require.Len(t, tags, 4)

		// Verify OTel tags have correct provider, version (where applicable), and explicit types
		assert.Equal(t, "db.query.text", tags[0].Name)
		assert.Equal(t, ProviderOTel, tags[0].Provider)
		assert.Equal(t, "1.24.0", tags[0].Version)
		assert.Equal(t, ValueTypeString, tags[0].Type)

		assert.Equal(t, "db.statement", tags[1].Name)
		assert.Equal(t, ProviderOTel, tags[1].Provider)
		assert.Empty(t, tags[1].Version) // deprecated, version not specified
		assert.Equal(t, ValueTypeString, tags[1].Type)

		// Verify Datadog tags have correct provider and explicit types
		assert.Equal(t, "sql.query", tags[2].Name)
		assert.Equal(t, ProviderDatadog, tags[2].Provider)
		assert.Equal(t, ValueTypeString, tags[2].Type)

		assert.Equal(t, "mongodb.query", tags[3].Name)
		assert.Equal(t, ProviderDatadog, tags[3].Provider)
		assert.Equal(t, ValueTypeString, tags[3].Type)
	})

	t.Run("_dd.top_level metadata", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptDDTopLevel)
		require.NotNil(t, tags)
		require.Len(t, tags, 2)

		assert.Equal(t, "_dd.top_level", tags[0].Name)
		assert.Equal(t, ProviderDatadog, tags[0].Provider)
		assert.Equal(t, ValueTypeFloat64, tags[0].Type)

		assert.Equal(t, "_top_level", tags[1].Name)
		assert.Equal(t, ProviderDatadog, tags[1].Provider)
		assert.Equal(t, ValueTypeFloat64, tags[1].Type)
	})
}

func TestTypeMetadata(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("http.status_code type chain", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptHTTPStatusCode)
		require.NotNil(t, tags)
		require.Len(t, tags, 3)

		// First: int64 type (numeric in Metrics)
		assert.Equal(t, "http.status_code", tags[0].Name)
		assert.Equal(t, ValueTypeInt64, tags[0].Type)

		// Second: string type (in Meta)
		assert.Equal(t, "http.status_code", tags[1].Name)
		assert.Equal(t, ValueTypeString, tags[1].Type)

		// Third: OTel string type (in Meta) with version
		assert.Equal(t, "http.response.status_code", tags[2].Name)
		assert.Equal(t, "1.21.0", tags[2].Version)
		assert.Equal(t, ValueTypeString, tags[2].Type)
	})

	t.Run("rpc.grpc.status_code type chain", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptGRPCStatusCode)
		require.NotNil(t, tags)
		require.Len(t, tags, 6)

		// First 4 should be int64 type (in Metrics)
		for i := 0; i < 4; i++ {
			assert.Equal(t, ValueTypeInt64, tags[i].Type, "tag[%d] should be int64 type", i)
		}

		// Last 2 should be string type (in Meta)
		for i := 4; i < 6; i++ {
			assert.Equal(t, ValueTypeString, tags[i].Type, "tag[%d] should be string type", i)
		}
	})
}

func TestVersionMetadata(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("stable HTTP attributes have version 1.21.0", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptHTTPMethod)
		require.NotNil(t, tags)
		require.Len(t, tags, 2)

		// http.request.method is stable in 1.21.0
		assert.Equal(t, "http.request.method", tags[0].Name)
		assert.Equal(t, "1.21.0", tags[0].Version)

		// http.method is deprecated, no version
		assert.Equal(t, "http.method", tags[1].Name)
		assert.Empty(t, tags[1].Version)
	})

	t.Run("stable DB attributes have version 1.24.0", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptDBQuery)
		require.NotNil(t, tags)

		// db.query.text is stable in 1.24.0
		assert.Equal(t, "db.query.text", tags[0].Name)
		assert.Equal(t, "1.24.0", tags[0].Version)

		// db.statement is deprecated, no version
		assert.Equal(t, "db.statement", tags[1].Name)
		assert.Empty(t, tags[1].Version)
	})

	t.Run("network attributes have version 1.21.0", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptPeerHostname)
		require.NotNil(t, tags)

		// Find network.destination.name and server.address
		var networkDestName, serverAddr *TagInfo
		for i := range tags {
			if tags[i].Name == "network.destination.name" {
				networkDestName = &tags[i]
			}
			if tags[i].Name == "server.address" {
				serverAddr = &tags[i]
			}
		}

		require.NotNil(t, networkDestName, "expected network.destination.name in fallbacks")
		assert.Equal(t, "1.21.0", networkDestName.Version)

		require.NotNil(t, serverAddr, "expected server.address in fallbacks")
		assert.Equal(t, "1.21.0", serverAddr.Version)
	})
}

func TestProviderMetadata(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("peer.hostname providers", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptPeerHostname)
		require.NotNil(t, tags)

		// Verify Datadog-specific attributes have datadog provider
		datadogAttrs := map[string]bool{
			"peer.hostname":    true,
			"hostname":         true,
			"db.hostname":      true,
			"grpc.host":        true,
			"http.host":        true,
			"http.server_name": true,
			"out.host":         true,
			"dns.hostname":     true,
		}

		// Verify OTel standard attributes have otel provider
		otelAttrs := map[string]bool{
			"net.peer.name":            true,
			"network.destination.name": true,
			"server.address":           true,
			"network.destination.ip":   true,
		}

		for _, tag := range tags {
			if datadogAttrs[tag.Name] {
				assert.Equal(t, ProviderDatadog, tag.Provider, "attribute %s should have datadog provider", tag.Name)
			}
			if otelAttrs[tag.Name] {
				assert.Equal(t, ProviderOTel, tag.Provider, "attribute %s should have otel provider", tag.Name)
			}
		}
	})
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	require.NotNil(t, r)

	tags := r.GetAttributePrecedence(ConceptDBQuery)
	assert.NotNil(t, tags)

	version := r.Version()
	assert.NotEmpty(t, version)
}

func TestNewRegistryFromJSON(t *testing.T) {
	customJSON := []byte(`{
		"version": "custom-1.0",
		"concepts": {
			"test.concept": {
				"canonical": "test.concept",
				"fallbacks": [
					{"name": "test.attr1", "provider": "datadog", "type": "string"},
					{"name": "test.attr2", "provider": "otel", "version": "1.0.0", "type": "string"}
				]
			}
		}
	}`)

	r, err := NewRegistryFromJSON(customJSON)
	require.NoError(t, err)
	require.NotNil(t, r)

	assert.Equal(t, "custom-1.0", r.Version())

	tags := r.GetAttributePrecedence(Concept("test.concept"))
	require.NotNil(t, tags)
	require.Len(t, tags, 2)
	assert.Equal(t, "test.attr1", tags[0].Name)
	assert.Equal(t, ProviderDatadog, tags[0].Provider)
	assert.Equal(t, ValueTypeString, tags[0].Type)
	assert.Equal(t, "test.attr2", tags[1].Name)
	assert.Equal(t, ProviderOTel, tags[1].Provider)
	assert.Equal(t, "1.0.0", tags[1].Version)
	assert.Equal(t, ValueTypeString, tags[1].Type)
}

func TestNewRegistryFromJSON_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)

	r, err := NewRegistryFromJSON(invalidJSON)
	assert.Error(t, err)
	assert.Nil(t, r)
}

func TestConcurrentAccess(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = r.GetAttributePrecedence(ConceptDBQuery)
				_ = r.GetAllEquivalences()
				_ = r.Version()
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmark tests
func BenchmarkGetAttributePrecedence(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.GetAttributePrecedence(ConceptDBQuery)
	}
}

func BenchmarkGetAllEquivalences(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.GetAllEquivalences()
	}
}
