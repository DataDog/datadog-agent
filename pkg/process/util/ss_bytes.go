package util

import (
	"bytes"
	"sort"
)

// SSBytes implements the sort.Interface for the [][]byte type
type SSBytes [][]byte

var _ sort.Interface = SSBytes{}

func (ss SSBytes) Len() int {
	return len(ss)
}

func (ss SSBytes) Less(i, j int) bool {
	return bytes.Compare(ss[i], ss[j]) < 0
}

func (ss SSBytes) Swap(i, j int) {
	ss[i], ss[j] = ss[j], ss[i]
}

// Search returns the index of element x if found or the length of the SSBytes otherwise.
// SSBytes is expected to be sorted.
func (ss SSBytes) Search(x []byte) int {
	i := sort.Search(len(ss), func(i int) bool {
		return bytes.Compare(ss[i], x) >= 0
	})

	if i < len(ss) && bytes.Compare(ss[i], x) == 0 {
		return i
	}

	return len(ss)
}
