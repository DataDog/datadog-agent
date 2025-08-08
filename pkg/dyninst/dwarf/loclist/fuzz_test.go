package loclist

import "testing"

func FuzzParseInstructions(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte, ptrSize uint8, totalByteSize uint32) {
		_, err := ParseInstructions(data, ptrSize, totalByteSize)
		if err != nil {
			t.Skip("Failed to parse instructions")
		}
	})
}
