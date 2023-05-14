// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestSerialization(t *testing.T) {
	tcs := []struct {
		origin map[int32]*procutil.StatsWithPerm
		exp    *model.ProcStatsWithPermByPID
	}{
		{
			origin: map[int32]*procutil.StatsWithPerm{
				1: {
					OpenFdCount: 1,
					IOStat: &procutil.IOCountersStat{
						ReadCount:  1,
						WriteCount: 2,
						ReadBytes:  3,
						WriteBytes: 4,
					},
				},
			},
			exp: &model.ProcStatsWithPermByPID{
				StatsByPID: map[int32]*model.ProcStatsWithPerm{
					1: {
						OpenFDCount: 1,
						ReadCount:   1,
						WriteCount:  2,
						ReadBytes:   3,
						WriteBytes:  4,
					},
				},
			},
		},
		{
			origin: map[int32]*procutil.StatsWithPerm{
				1: {
					OpenFdCount: 2,
					IOStat: &procutil.IOCountersStat{
						ReadCount:  4,
						WriteCount: 2,
						ReadBytes:  5,
						WriteBytes: 8,
					},
				},
			},
			exp: &model.ProcStatsWithPermByPID{
				StatsByPID: map[int32]*model.ProcStatsWithPerm{
					1: {
						OpenFDCount: 2,
						ReadCount:   4,
						WriteCount:  2,
						ReadBytes:   5,
						WriteBytes:  8,
					},
				},
			},
		},
	}

	t.Run("requesting application/json serialization", func(t *testing.T) {
		marshaler := GetMarshaler("application/json")
		assert.Equal(t, "application/json", marshaler.ContentType())
		unmarshaler := GetUnmarshaler("application/json")

		for _, tc := range tcs {
			blob, err := marshaler.Marshal(tc.origin)
			require.NoError(t, err)

			result, err := unmarshaler.Unmarshal(blob)
			require.NoError(t, err)
			assert.Equal(t, tc.exp, result)
		}
	})

	t.Run("requesting empty marshaler name serialization", func(t *testing.T) {
		marshaler := GetMarshaler("")
		// in case we request empty serialization type, default to application/json
		assert.Equal(t, "application/json", marshaler.ContentType())
		unmarshaler := GetUnmarshaler("application/json")

		for _, tc := range tcs {
			blob, err := marshaler.Marshal(tc.origin)
			require.NoError(t, err)

			result, err := unmarshaler.Unmarshal(blob)
			require.NoError(t, err)
			assert.Equal(t, tc.exp, result)
		}
	})

	t.Run("requesting application/protobuf serialization", func(t *testing.T) {
		marshaler := GetMarshaler("application/protobuf")
		assert.Equal(t, "application/protobuf", marshaler.ContentType())
		unmarshaler := GetUnmarshaler("application/protobuf")

		for _, tc := range tcs {
			blob, err := marshaler.Marshal(tc.origin)
			require.NoError(t, err)

			result, err := unmarshaler.Unmarshal(blob)
			require.NoError(t, err)
			assert.Equal(t, tc.exp, result)
		}

	})

	t.Run("protobuf serializing empty input", func(t *testing.T) {
		marshaler := GetMarshaler("application/protobuf")
		assert.Equal(t, "application/protobuf", marshaler.ContentType())
		unmarshaler := GetUnmarshaler("application/protobuf")

		var empty map[int32]*procutil.StatsWithPerm
		blob, err := marshaler.Marshal(empty)
		require.NoError(t, err)

		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.EqualValues(t, &model.ProcStatsWithPermByPID{}, result)
	})

	t.Run("json serializing empty input", func(t *testing.T) {
		marshaler := GetMarshaler("application/json")
		assert.Equal(t, "application/json", marshaler.ContentType())

		var empty map[int32]*procutil.StatsWithPerm
		blob, err := marshaler.Marshal(empty)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.EqualValues(t, &model.ProcStatsWithPermByPID{StatsByPID: map[int32]*model.ProcStatsWithPerm{}}, result)
	})
}
