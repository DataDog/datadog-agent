// +build zstd

package compression

import "github.com/DataDog/zstd"

// Compress will compress the data with zstd
func Compress(dst []byte, src []byte) ([]byte, error) {
	return zstd.Compress(dst, src)
}
