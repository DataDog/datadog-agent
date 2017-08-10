// +build zstd

package compression

import "github.com/DataDog/zstd"

// TODO: the intake still uses a pre-v1 (unstable) version of the zstd compression format.
// The agent shouldn't use zstd compression until the intake supports a stable v1 format.

// ContentEncoding describes the HTTP header value associated with the compression method
// var instead of const to ease testing
var ContentEncoding = "zstd"

// Compress will compress the data with zstd
func Compress(dst []byte, src []byte) ([]byte, error) {
	return zstd.Compress(dst, src)
}
