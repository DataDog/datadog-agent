package cache

import (
	"errors"
)

const MaxValueSize = 4080

// Stub implementation for mmap_hash for non-Linux platforms.

func Report() {}

type mmapHash struct {
}

func (*mmapHash) Name() string {
	return "unimplemented"
}

func (*mmapHash) lookupOrInsert(key []byte) (string, bool) {
	return string(key), false
}

func (*mmapHash) finalize() {
}

func (*mmapHash) sizes() (int64, int64) {
	return 0, 0
}

func newMmapHash(origin string, fileSize int64, prefixPath string, closeOnRelease bool) (*mmapHash, error) {
	return nil, errors.New("unsupported platform for mmap hash")
}

func CheckKnown(tag string) string {
	return tag
}

func Check(tag string) bool {
	return true
}

func CheckDefault(tag string) string {
	return tag
}
