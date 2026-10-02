// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSDSResultAggregator(t *testing.T) {
	t.Run("ParseSDSResult should return empty array on empty data", func(t *testing.T) {
		payloads, err := ParseSDSResult(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("ParseSDSResult should ignore empty JSON connectivity probe", func(t *testing.T) {
		payloads, err := ParseSDSResult(api.Payload{
			Data:        []byte("{}"),
			Encoding:    encodingJSON,
			ContentType: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("ParseSDSResult should parse a valid payload", func(t *testing.T) {
		collectedTime := time.Now()
		msg := newSDSResultProto()
		payloads, err := ParseSDSResult(api.Payload{
			Data:      mustMarshalSDSResult(t, msg),
			Encoding:  encodingEmpty,
			Timestamp: collectedTime,
		})
		require.NoError(t, err)
		require.Len(t, payloads, 1)

		payload := payloads[0]
		assert.Equal(t, "task-1:sub-1", payload.name())
		assert.Equal(t, collectedTime, payload.GetCollectedTime())
		assert.Empty(t, payload.GetTags())
		require.True(t, proto.Equal(msg, &payload.SdsResultPayload), "parsed payload should match the proto")
	})

	t.Run("name should be unknown when scan results are missing", func(t *testing.T) {
		payload := &SDSResultPayload{}
		assert.Equal(t, "unknown", payload.name())
	})
}

func newSDSResultProto() *sds.SdsResultPayload {
	return &sds.SdsResultPayload{
		Timestamp: 1_700_000_000_000,
		Resource: &sds.SdsResultPayload_Resource{
			Type: "postgres_table",
			Name: "inst.app.public.users",
		},
		RuleIds: []string{"email"},
		ScanningSource: &sds.ScanningSource{
			Source: &sds.ScanningSource_Agent_{
				Agent: &sds.ScanningSource_Agent{},
			},
		},
		ScanResults: []*sds.SdsResultPayload_ScanResult{{
			TableMatches: []*sds.SdsResultPayload_TableMatch{{
				RuleId:           "email",
				ColumnName:       "email",
				CountMatchedRows: 2,
				CountMatches:     3,
			}},
			Location: &sds.SdsResultPayload_ScanLocation{
				ScanLocation: &sds.SdsResultPayload_ScanLocation_PostgresTable{
					PostgresTable: &sds.SdsResultPayload_PostgresTable{
						DatabaseClusterName:  "cluster",
						DatabaseInstanceName: "inst",
						DatabaseHostName:     "localhost",
						DatabaseName:         "app",
						SchemaName:           "public",
						TableName:            "users",
						ScannedRowCount:      2,
						ScannedColumns: []*sds.SdsResultPayload_PostgresTable_ScannedColumn{{
							Name:     "email",
							DataType: "text",
						}},
					},
				},
			},
			ScanMetadata: &sds.SdsResultPayload_ScanMetadata{
				ScanTaskMetadata: &sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata{
					TaskId:    "task-1",
					SubTaskId: "sub-1",
					Status:    sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata_SUCCESS,
				},
			},
		}},
	}
}

func mustMarshalSDSResult(t *testing.T, msg *sds.SdsResultPayload) []byte {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	return data
}
