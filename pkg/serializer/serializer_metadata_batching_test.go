// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib && zstd

package serializer

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer/limits"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// largeMetadataPayload is a test payload that generates metadata exceeding the limit
type largeMetadataPayload struct {
	uuid          string
	checkMetadata map[string][]map[string]interface{}
}

func (p *largeMetadataPayload) MarshalJSON() ([]byte, error) {
	payload := map[string]interface{}{
		"uuid":           p.uuid,
		"check_metadata": p.checkMetadata,
	}
	return json.Marshal(payload)
}

// generateLargeCheckMetadata creates check_metadata that will exceed the given size
// Uses pre-computed sizes for efficiency
func generateLargeCheckMetadata(targetSize int) map[string][]map[string]interface{} {
	checkMetadata := make(map[string][]map[string]interface{})

	// Each instance is approximately 350 bytes when serialized
	// Each check with 10 instances is approximately 3500 bytes
	instancesPerCheck := 10
	estimatedSizePerCheck := 3500

	numChecks := (targetSize / estimatedSizePerCheck) + 1

	for i := 0; i < numChecks; i++ {
		checkName := "check_" + strings.Repeat("x", 10) + "_" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
		instances := make([]map[string]interface{}, 0, instancesPerCheck)

		for j := 0; j < instancesPerCheck; j++ {
			instance := map[string]interface{}{
				"config.hash":     strings.Repeat("h", 40),
				"config.provider": "file",
				"init_config": map[string]interface{}{
					"key":   strings.Repeat("v", 50),
					"other": strings.Repeat("d", 50),
				},
				"instance_id": j,
				"extra_field": strings.Repeat("x", 100),
			}
			instances = append(instances, instance)
		}

		checkMetadata[checkName] = instances
	}

	return checkMetadata
}

// TestMetadataBatchingRequired demonstrates that large payloads exceed the limit (RED test)
// This test verifies the PROBLEM exists: payloads >1MB would be rejected by intake
func TestMetadataBatchingRequired(t *testing.T) {
	// Generate a payload that exceeds 1MB
	checkMetadata := generateLargeCheckMetadata(1500 * 1024) // 1.5MB of check_metadata

	payload := &largeMetadataPayload{
		uuid:          "test-uuid-12345",
		checkMetadata: checkMetadata,
	}

	jsonBytes, err := payload.MarshalJSON()
	require.NoError(t, err)

	// RED: Verify the payload exceeds the intake limit
	assert.Greater(t, len(jsonBytes), limits.MetadataMaxUncompressed,
		"Test payload should exceed 1MB limit to demonstrate the problem")
}

// TestMetadataBatchingSplitsLargePayloads verifies batching splits payloads correctly (GREEN test)
func TestMetadataBatchingSplitsLargePayloads(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			f := &forwarder.MockedForwarder{}
			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

			// Generate a payload that requires batching (~1.2MB)
			checkMetadata := generateLargeCheckMetadata(1200 * 1024)
			payload := &largeMetadataPayload{
				uuid:          "test-uuid-batch",
				checkMetadata: checkMetadata,
			}

			// Track submitted payloads
			var submittedPayloads [][]byte
			f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				payloads := args.Get(0).(transaction.BytesPayloads)
				for _, p := range payloads {
					decompressed, err := s.Strategy.Decompress(p.GetContent())
					require.NoError(t, err)
					submittedPayloads = append(submittedPayloads, decompressed)
				}
			}).Return(nil)

			// Send the large payload
			err := s.SendMetadata(payload)
			require.NoError(t, err)

			// GREEN: Verify multiple batches were created
			require.Greater(t, len(submittedPayloads), 1,
				"Large payload should be split into multiple batches")

			// Verify each batch is under the limit
			for i, batch := range submittedPayloads {
				assert.LessOrEqual(t, len(batch), limits.MetadataMaxUncompressed,
					"Batch %d should be under 1MB limit", i)
			}

			// Verify batch metadata is present and correct
			for i, batch := range submittedPayloads {
				var batchMap map[string]interface{}
				err := json.Unmarshal(batch, &batchMap)
				require.NoError(t, err, "Batch %d should be valid JSON", i)

				// Verify UUID is preserved
				assert.Equal(t, "test-uuid-batch", batchMap["uuid"],
					"Batch %d should preserve original UUID", i)

				// Verify batch index and total are present
				batchIndex, hasIndex := batchMap["_dd_batch_index"]
				batchTotal, hasTotal := batchMap["_dd_batch_total"]
				assert.True(t, hasIndex, "Batch %d should have _dd_batch_index", i)
				assert.True(t, hasTotal, "Batch %d should have _dd_batch_total", i)

				// Verify batch index matches position
				assert.Equal(t, float64(i), batchIndex,
					"Batch %d should have correct index", i)

				// Verify batch total matches number of batches
				assert.Equal(t, float64(len(submittedPayloads)), batchTotal,
					"Batch %d should have correct total", i)
			}

			f.AssertExpectations(t)
		})
	}
}

// TestMetadataSmallPayloadNotBatched verifies small payloads are sent as-is (fast path)
func TestMetadataSmallPayloadNotBatched(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	// Create a small payload (well under 800KB)
	payload := &largeMetadataPayload{
		uuid: "small-uuid",
		checkMetadata: map[string][]map[string]interface{}{
			"cpu": {{
				"config.hash": "abc123",
			}},
		},
	}

	var submittedPayloads [][]byte
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		payloads := args.Get(0).(transaction.BytesPayloads)
		for _, p := range payloads {
			decompressed, err := s.Strategy.Decompress(p.GetContent())
			require.NoError(t, err)
			submittedPayloads = append(submittedPayloads, decompressed)
		}
	}).Return(nil)

	err := s.SendMetadata(payload)
	require.NoError(t, err)

	// Should be exactly one payload (no batching)
	require.Len(t, submittedPayloads, 1)

	// Should NOT have batch metadata
	var resultMap map[string]interface{}
	err = json.Unmarshal(submittedPayloads[0], &resultMap)
	require.NoError(t, err)

	_, hasIndex := resultMap["_dd_batch_index"]
	_, hasTotal := resultMap["_dd_batch_total"]
	assert.False(t, hasIndex, "Small payload should not have batch index")
	assert.False(t, hasTotal, "Small payload should not have batch total")

	f.AssertExpectations(t)
}

// TestMetadataBatchingDeterministic verifies batching produces deterministic results
func TestMetadataBatchingDeterministic(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	// Create the same payload twice and verify batches are identical
	checkMetadata := map[string][]map[string]interface{}{
		"zebra": {{"id": 1}, {"id": 2}},
		"alpha": {{"id": 3}, {"id": 4}},
		"mango": {{"id": 5}, {"id": 6}},
	}

	var allBatches [][]string
	for run := 0; run < 2; run++ {
		f := &forwarder.MockedForwarder{}
		compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
		s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

		// Use a small target size to force batching
		payload := &largeMetadataPayload{
			uuid:          "deterministic-test",
			checkMetadata: checkMetadata,
		}

		var thisRunPayloads []string
		f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			payloads := args.Get(0).(transaction.BytesPayloads)
			for _, p := range payloads {
				decompressed, err := s.Strategy.Decompress(p.GetContent())
				require.NoError(t, err)
				thisRunPayloads = append(thisRunPayloads, string(decompressed))
			}
		}).Return(nil)

		err := s.SendMetadata(payload)
		require.NoError(t, err)
		allBatches = append(allBatches, thisRunPayloads)
	}

	// Both runs should produce identical results
	require.Equal(t, allBatches[0], allBatches[1],
		"Batching should be deterministic across runs")
}

// TestMetadataBatchingEmptyCheckMetadata verifies empty check_metadata is handled
func TestMetadataBatchingEmptyCheckMetadata(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	payload := &largeMetadataPayload{
		uuid:          "empty-metadata",
		checkMetadata: map[string][]map[string]interface{}{},
	}

	var callCount int
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		callCount++
	}).Return(nil)

	err := s.SendMetadata(payload)
	require.NoError(t, err)

	// Should still send one payload (the original)
	assert.Equal(t, 1, callCount, "Empty check_metadata should still send one payload")
	f.AssertExpectations(t)
}

// TestMetadataBatchingNoCheckMetadata verifies payloads without check_metadata are handled
func TestMetadataBatchingNoCheckMetadata(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	// Payload without check_metadata field
	type noCheckMetadataPayload struct {
		UUID     string `json:"uuid"`
		Hostname string `json:"hostname"`
	}

	payload := &simpleJSONPayload{data: noCheckMetadataPayload{
		UUID:     "no-check-metadata",
		Hostname: "testhost",
	}}

	var callCount int
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		callCount++
	}).Return(nil)

	err := s.SendMetadata(payload)
	require.NoError(t, err)

	assert.Equal(t, 1, callCount, "Payload without check_metadata should send one payload")
	f.AssertExpectations(t)
}

// TestMetadataBatchingSubmitError verifies errors are propagated correctly
func TestMetadataBatchingSubmitError(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	// Generate a payload that requires batching
	checkMetadata := generateLargeCheckMetadata(1200 * 1024)
	payload := &largeMetadataPayload{
		uuid:          "error-test",
		checkMetadata: checkMetadata,
	}

	// First batch succeeds, second fails
	callCount := 0
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		callCount++
	}).Return(nil).Once()

	f.On("SubmitMetadata", mock.Anything, mock.Anything).Return(
		assert.AnError,
	).Once()

	err := s.SendMetadata(payload)
	require.Error(t, err, "Should return error when batch submission fails")
	assert.Contains(t, err.Error(), "failed to submit metadata batch")
}

// simpleJSONPayload wraps any struct for JSON marshaling
type simpleJSONPayload struct {
	data interface{}
}

func (p *simpleJSONPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.data)
}

// TestMetadataBatchingPreservesAllFields verifies all original fields are preserved
func TestMetadataBatchingPreservesAllFields(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	// Payload with multiple fields besides check_metadata
	type fullMetadataPayload struct {
		UUID          string                              `json:"uuid"`
		InternalIP    string                              `json:"internal_ip"`
		Hostname      string                              `json:"hostname"`
		AgentVersion  string                              `json:"agent_version"`
		CheckMetadata map[string][]map[string]interface{} `json:"check_metadata"`
	}

	// Generate enough check_metadata to trigger batching
	checkMetadata := generateLargeCheckMetadata(1000 * 1024)

	payload := &simpleJSONPayload{data: fullMetadataPayload{
		UUID:          "preserve-fields-uuid",
		InternalIP:    "10.0.0.1",
		Hostname:      "test-host",
		AgentVersion:  "7.50.0",
		CheckMetadata: checkMetadata,
	}}

	var submittedPayloads [][]byte
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		payloads := args.Get(0).(transaction.BytesPayloads)
		for _, p := range payloads {
			decompressed, err := s.Strategy.Decompress(p.GetContent())
			require.NoError(t, err)
			submittedPayloads = append(submittedPayloads, decompressed)
		}
	}).Return(nil)

	err := s.SendMetadata(payload)
	require.NoError(t, err)

	// Verify all batches preserve original fields
	for i, batch := range submittedPayloads {
		var batchMap map[string]interface{}
		err := json.Unmarshal(batch, &batchMap)
		require.NoError(t, err)

		assert.Equal(t, "preserve-fields-uuid", batchMap["uuid"], "Batch %d: uuid", i)
		assert.Equal(t, "10.0.0.1", batchMap["internal_ip"], "Batch %d: internal_ip", i)
		assert.Equal(t, "test-host", batchMap["hostname"], "Batch %d: hostname", i)
		assert.Equal(t, "7.50.0", batchMap["agent_version"], "Batch %d: agent_version", i)
	}

	f.AssertExpectations(t)
}

// TestMetadataBatchingDropsOversizedEntries verifies entries exceeding 1MB are dropped with a warning
func TestMetadataBatchingDropsOversizedEntries(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	logger := logmock.New(t)
	s := NewSerializer(f, nil, compressor, mockConfig, logger, "testhost")

	// Create an entry that exceeds 1MB (MetadataMaxUncompressed)
	oversizedData := strings.Repeat("x", limits.MetadataMaxUncompressed+1000)
	oversizedEntry := map[string]interface{}{
		"config.hash": oversizedData,
	}

	// Create a normal-sized entry
	normalEntry := map[string]interface{}{
		"config.hash":     "normal-hash",
		"config.provider": "file",
	}

	// Payload with one oversized check and one normal check
	payload := &largeMetadataPayload{
		uuid: "oversized-test",
		checkMetadata: map[string][]map[string]interface{}{
			"oversized_check": {oversizedEntry},
			"normal_check":    {normalEntry},
		},
	}

	// Verify the oversized entry actually exceeds the limit
	oversizedJSON, _ := json.Marshal(map[string]interface{}{"oversized_check": []map[string]interface{}{oversizedEntry}})
	require.Greater(t, len(oversizedJSON), limits.MetadataMaxUncompressed,
		"Test setup: oversized entry should exceed 1MB limit")

	var submittedPayloads [][]byte
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		payloads := args.Get(0).(transaction.BytesPayloads)
		for _, p := range payloads {
			decompressed, err := s.Strategy.Decompress(p.GetContent())
			require.NoError(t, err)
			submittedPayloads = append(submittedPayloads, decompressed)
		}
	}).Return(nil)

	err := s.SendMetadata(payload)
	require.NoError(t, err, "SendMetadata should succeed even when dropping oversized entries")

	// Verify at least one payload was sent
	require.NotEmpty(t, submittedPayloads, "Should send at least one payload with normal entries")

	// Verify the oversized check was dropped and normal check was included
	for _, batch := range submittedPayloads {
		var batchMap map[string]interface{}
		err := json.Unmarshal(batch, &batchMap)
		require.NoError(t, err)

		checkMetadata, ok := batchMap["check_metadata"].(map[string]interface{})
		if !ok {
			continue
		}

		// Oversized check should NOT be present in any batch
		_, hasOversized := checkMetadata["oversized_check"]
		assert.False(t, hasOversized, "Oversized entry should be dropped from all batches")

		// Normal check should be present
		if _, hasNormal := checkMetadata["normal_check"]; hasNormal {
			// Verify normal entry content is preserved
			normalInstances := checkMetadata["normal_check"].([]interface{})
			require.Len(t, normalInstances, 1)
			normalInstance := normalInstances[0].(map[string]interface{})
			assert.Equal(t, "normal-hash", normalInstance["config.hash"])
		}
	}

	// Verify at least one batch contains the normal check
	foundNormal := false
	for _, batch := range submittedPayloads {
		if strings.Contains(string(batch), "normal_check") {
			foundNormal = true
			break
		}
	}
	assert.True(t, foundNormal, "Normal entry should be present in at least one batch")

	f.AssertExpectations(t)
}

// TestMetadataBatchingExtraHeaders verifies correct headers are sent
func TestMetadataBatchingExtraHeaders(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", compression.ZlibKind)

	f := &forwarder.MockedForwarder{}
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	checkMetadata := generateLargeCheckMetadata(1000 * 1024)
	payload := &largeMetadataPayload{
		uuid:          "headers-test",
		checkMetadata: checkMetadata,
	}

	var capturedHeaders []http.Header
	f.On("SubmitMetadata", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		headers := args.Get(1).(http.Header)
		capturedHeaders = append(capturedHeaders, headers)
	}).Return(nil)

	err := s.SendMetadata(payload)
	require.NoError(t, err)

	// All batches should have compression headers
	for i, headers := range capturedHeaders {
		assert.Equal(t, "application/json", headers.Get("Content-Type"),
			"Batch %d should have JSON content type", i)
		assert.NotEmpty(t, headers.Get("Content-Encoding"),
			"Batch %d should have content encoding", i)
	}

	f.AssertExpectations(t)
}
