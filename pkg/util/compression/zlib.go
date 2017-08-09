// +build zlib

package compression

import (
	"bytes"
	"compress/zlib"
)

// ContentEncoding describes the HTTP header value associated with the compression method
// var instead of const to ease testing
var ContentEncoding = "deflate"

// Compress will compress the data with zlib
func Compress(dst []byte, src []byte) ([]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(src)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	dst = b.Bytes()
	return dst, nil
}
