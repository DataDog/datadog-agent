// +build zlib

package compression

import (
	"bytes"
	"compress/zlib"
)

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
