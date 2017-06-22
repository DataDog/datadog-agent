// +build !zlib,!zstd

package compression

// Compress will not compress anything
func Compress(dst []byte, src []byte) ([]byte, error) {
	dst = src
	return dst, nil
}
