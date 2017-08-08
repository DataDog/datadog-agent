// +build !zlib,!zstd

package compression

// ContentEncoding describes the HTTP header value associated with the compression
// empty here since there's no compression
const ContentEncoding = ""

// Compress will not compress anything
func Compress(dst []byte, src []byte) ([]byte, error) {
	dst = src
	return dst, nil
}
