// +build linux_bpf

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath(t *testing.T) {
	tx := httpTX{
		request_fragment: requestFragment(
			[]byte("GET /foo/bar?var1=value HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"),
		),
	}

	assert.Equal(t, "/foo/bar", tx.Path())
}

func requestFragment(fragment []byte) [HTTPBufferSize]_Ctype_char {
	var b [HTTPBufferSize]_Ctype_char
	for i := 0; i < len(b) && i < len(fragment); i++ {
		b[i] = _Ctype_char(fragment[i])
	}
	return b
}
