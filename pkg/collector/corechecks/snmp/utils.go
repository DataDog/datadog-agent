package snmp

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
)

func createStringBatches(elements []string, size int) ([][]string, error) {
	var batches [][]string

	if size <= 0 {
		return nil, fmt.Errorf("batch size must be positive. invalid size: %d", size)
	}

	for i := 0; i < len(elements); i += size {
		j := i + size
		if j > len(elements) {
			j = len(elements)
		}
		batch := elements[i:j]
		batches = append(batches, batch)
	}

	return batches, nil
}

func copyStrings(tags []string) []string {
	newTags := make([]string, len(tags))
	copy(newTags, tags)
	return newTags
}

func buildDeviceID(tags []string) (string, []string) {
	h := fnv.New64()
	idTags := copyStrings(tags)
	sort.Strings(idTags)
	for _, tag := range tags {
		// the implementation of h.Write never returns a non-nil error
		_, _ = h.Write([]byte(tag))
	}
	return strconv.FormatUint(h.Sum64(), 16), idTags
}
