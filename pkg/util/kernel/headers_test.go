// +build linux

package kernel

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindHeaderDirs(t *testing.T) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		t.Skip("set INTEGRATION environment variable to run")
	}
	dirs, err := FindHeaderDirs()
	require.NoError(t, err)
	assert.NotZero(t, len(dirs), "expected to find header directories")
	t.Log(dirs)
}

func TestParseHeaderVersion(t *testing.T) {
	cases := []struct {
		body string
		v    Version
		err  bool
	}{
		{"#define LINUX_VERSION_CODE 328769", Version(328769), false},
		{"#define  LINUX_VERSION_CODE		123456", Version(123456), false},
		{"#define LINUX_VERSION_CODE -1", Version(0), true},
		{"#define LINUX_VERSION_CODE", Version(0), true},
		{"", Version(0), true},
	}

	for _, c := range cases {
		hv, err := parseHeaderVersion(bytes.NewBufferString(c.body))
		if c.err {
			assert.Error(t, err, "expected error parsing of `%s`", c.body)
		} else {
			if assert.NoError(t, err, "parse error of `%s`", c.body) {
				assert.Equal(t, c.v, hv, "version mismatch of `%s`", c.body)
			}
		}
	}
}
