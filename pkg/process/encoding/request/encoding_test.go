package request

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

func TestSerialization(t *testing.T) {
	tcs := []struct {
		req *pbgo.ProcessStatRequest
		exp *pbgo.ProcessStatRequest
	}{
		{
			req: &pbgo.ProcessStatRequest{
				Pids: []int32{1, 2, 3},
			},
			exp: &pbgo.ProcessStatRequest{
				Pids: []int32{1, 2, 3},
			},
		},
	}

	t.Run("requesting application/json serialization", func(t *testing.T) {
		marshaler := GetMarshaler("application/json")
		assert.Equal(t, "application/json", marshaler.ContentType())
		unmarshaler := GetUnmarshaler("application/json")

		for _, tc := range tcs {
			blob, err := marshaler.Marshal(tc.req)
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
			blob, err := marshaler.Marshal(tc.req)
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
			blob, err := marshaler.Marshal(tc.req)
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

		blob, err := marshaler.Marshal(&pbgo.ProcessStatRequest{})
		require.NoError(t, err)

		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.EqualValues(t, &pbgo.ProcessStatRequest{}, result)
	})

	t.Run("json serializing empty input", func(t *testing.T) {
		marshaler := GetMarshaler("application/json")
		assert.Equal(t, "application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(&pbgo.ProcessStatRequest{})
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.EqualValues(t, &pbgo.ProcessStatRequest{Pids: []int32{}}, result)
	})
}
