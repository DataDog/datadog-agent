package snmp

import "fmt"

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
