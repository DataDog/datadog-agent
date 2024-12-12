// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGenerateSpanPointerHash(t *testing.T) {
	tests := []struct {
		name         string
		components   []string
		expectedHash string
	}{
		{
			name:         "basic values",
			components:   []string{"some-bucket", "some-key.data", "ab12ef34"},
			expectedHash: "e721375466d4116ab551213fdea08413",
		},
		{
			name:         "non-ascii key",
			components:   []string{"some-bucket", "some-key.你好", "ab12ef34"},
			expectedHash: "d1333a04b9928ab462b5c6cadfa401f4",
		},
		{
			name:         "multipart-upload",
			components:   []string{"some-bucket", "some-key.data", "ab12ef34-5"},
			expectedHash: "2b90dffc37ebc7bc610152c3dc72af9f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualHash := generateSpanPointerHash(tt.components)
			assert.Equal(t, tt.expectedHash, actualHash)
		})
	}
}

func TestAddSpanPointersFromS3Event(t *testing.T) {
	tests := []struct {
		name                 string
		event                events.S3Event
		expectedSpanPointers []SpanPointer
	}{
		{
			name: "single record",
			event: events.S3Event{
				Records: []events.S3EventRecord{
					{
						S3: events.S3Entity{
							Bucket: events.S3Bucket{Name: "test-bucket"},
							Object: events.S3Object{
								Key:  "test-key.data",
								ETag: "\"abc\"",
							},
						},
					},
				},
			},
			expectedSpanPointers: []SpanPointer{
				{
					Hash: "1e2a4f250ca4e4de7eab7da3b672515a",
					Kind: s3PointerKind,
				},
			},
		},
		{
			name: "single record with no etag quotes",
			event: events.S3Event{
				Records: []events.S3EventRecord{
					{
						S3: events.S3Entity{
							Bucket: events.S3Bucket{Name: "test-bucket"},
							Object: events.S3Object{
								Key:  "test-key.data",
								ETag: "abc",
							},
						},
					},
				},
			},
			expectedSpanPointers: []SpanPointer{
				{
					Hash: "1e2a4f250ca4e4de7eab7da3b672515a",
					Kind: s3PointerKind,
				},
			},
		},
		{
			name: "multiple invocations - should only keep most recent pointer",
			event: events.S3Event{
				Records: []events.S3EventRecord{
					{
						S3: events.S3Entity{
							Bucket: events.S3Bucket{Name: "test-bucket"},
							Object: events.S3Object{
								Key:  "test-key",
								ETag: "\"123456\"",
							},
						},
					},
					{
						S3: events.S3Entity{
							Bucket: events.S3Bucket{Name: "some-bucket"},
							Object: events.S3Object{
								Key:  "some-key.data",
								ETag: "\"ab12ef34\"",
							},
						},
					},
				},
			},
			expectedSpanPointers: []SpanPointer{
				{
					Hash: "e721375466d4116ab551213fdea08413",
					Kind: s3PointerKind,
				},
			},
		},
		{
			name:                 "empty event",
			event:                events.S3Event{},
			expectedSpanPointers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lp := &LifecycleProcessor{
				requestHandler: &RequestHandler{},
			}

			lp.addSpanPointersFromS3Event(tt.event)

			assert.Equal(t, tt.expectedSpanPointers, lp.requestHandler.spanPointers)
		})
	}
}
