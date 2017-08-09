// +build zstd

package compression

import "github.com/DataDog/zstd"

// ContentEncoding describes the HTTP header value associated with the compression method
// var instead of const to ease testing
var ContentEncoding = "zstd"

// Compress will compress the data with zstd
func Compress(dst []byte, src []byte) ([]byte, error) {
	return zstd.Compress(dst, src)
}
