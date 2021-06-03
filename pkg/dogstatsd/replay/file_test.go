package replay

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderFormat(t *testing.T) {
	buff := bytes.NewBuffer([]byte{})
	contents := bufio.NewWriter(buff)

	err := WriteHeader(contents)
	assert.Nil(t, err)

	// let's make sure these are written to the underlying byte buffer
	contents.Flush()

	// it should match the file-format
	b := buff.Bytes()
	assert.True(t, datadogMatcher(b))

	// look at version
	v, err := fileVersion(b)
	assert.Nil(t, err)
	assert.Equal(t, v, int(datadogFileVersion))

	// let's inspect the header
	for i := 0; i < len(datadogHeader); i++ {
		if i != versionIndex {
			assert.Equal(t, b[i], datadogHeader[i])
		} else {
			assert.Equal(t, b[i], datadogHeader[i]|datadogFileVersion)
		}
	}
}

func TestFormatMatcher(t *testing.T) {
	assert.True(t, datadogMatcher(datadogHeader))

	badDatadogHeader := []byte{0xD4, 0x74, 0xD0, 0x66, 0xF0, 0xFF, 0x00, 0x00}
	assert.False(t, datadogMatcher(badDatadogHeader))
}
