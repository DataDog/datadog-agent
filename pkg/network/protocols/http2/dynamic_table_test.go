package http2

import (
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	lru "github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2/hpack"
)

var (
	// pathExceedingMaxSize is path with size 166, which is exceeding the maximum path size in the kernel (HTTP2_MAX_PATH_LEN).
	pathExceedingMaxSize = "X2YRUwfeNEmYWkk0bACThVya8MoSUkR7ZKANCPYkIGHvF9CWGA0rxXKsGogQag7HsJfmgaar3TiOTRUb3ynbmiOz3As9rXYjRGNRdCWGgdBPL8nGa6WheGlJLNtIVsUcxSerNQKmoQqqDLjGftbKXjqdMJLVY6UyECeXOKrrFU9aHx2fjlk2qMNDUptYWuzPPCWAnKOV7Ph"
)

// generateRandomString creates a random ASCII string of a given size, ensuring it starts with '/'
func generateRandomString(size int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.New(rand.NewSource(time.Now().UnixNano()))

	if size == 0 {
		return "/"
	}

	result := make([]byte, size)
	result[0] = '/' // Ensure the first character is '/'

	for i := 1; i < size; i++ {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

// generateHuffmanEncodedString ensures the encoded output is exactly 'length' bytes.
// Since there's no direct way to control the output size of Huffman encoding, we need to guess what input
// string will produce the desired output size.
func generateHuffmanEncodedString(targetLength int) ([]byte, string) {
	estimate := targetLength
	var encoded []byte
	var original string

	for {
		original = generateRandomString(estimate)
		encoded = hpack.AppendHuffmanString(nil, original)

		if len(encoded) == targetLength {
			break
		} else if len(encoded) > targetLength {
			estimate-- // Reduce input size if output is too long
		} else {
			estimate++ // Increase input size if output is too short
		}
	}

	return encoded, original
}

func BenchmarkBla(b *testing.B) {
	dt := NewDynamicTable(config.New())
	var err error
	dt.dynamicTable, err = lru.NewLRU[HTTP2DynamicTableIndex, DynamicTableEntry](100000, nil)
	require.NoError(b, err)
	path, _ := generateHuffmanEncodedString(120)

	v := DynamicTableValue{
		Key: HTTP2DynamicTableIndex{
			Index: 0,
			Tup: ConnTuple{
				Saddr_h:  0,
				Saddr_l:  0,
				Daddr_h:  0,
				Daddr_l:  0,
				Sport:    0,
				Dport:    0,
				Netns:    0,
				Pid:      0,
				Metadata: 0,
			},
		},
		Value: HTTP2DynamicTableEntry{
			Buffer:             [160]uint8{},
			Value_type:         PathType,
			String_len:         120,
			Is_huffman_encoded: true,
		},
	}
	copy(v.Value.Buffer[:], path)

	values := make([]DynamicTableValue, DynamicTableBatchSize)
	for i := 0; i < DynamicTableBatchSize; i++ {
		values[i] = v
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		for j := 0; j < DynamicTableBatchSize; j++ {
			values[j].Key.Index = i*uint64(DynamicTableBatchSize) + uint64(j)
		}
		v.Key.Index = i
		dt.processDynamicTable(values)
	}
}
