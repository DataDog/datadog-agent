package snmp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_makeStringBatches(t *testing.T) {
	tests := []struct {
		name            string
		elements        []string
		size            int
		expectedBatches [][]string
		expectedError   error
	}{
		{
			"three batches, last with diff length",
			[]string{"aa", "bb", "cc", "dd", "ee"},
			2,
			[][]string{
				{"aa", "bb"},
				{"cc", "dd"},
				{"ee"},
			},
			nil,
		},
		{
			"two batches same length",
			[]string{"aa", "bb", "cc", "dd", "ee", "ff"},
			3,
			[][]string{
				{"aa", "bb", "cc"},
				{"dd", "ee", "ff"},
			},
			nil,
		},
		{
			"one full batch",
			[]string{"aa", "bb", "cc"},
			3,
			[][]string{
				{"aa", "bb", "cc"},
			},
			nil,
		},
		{
			"one partial batch",
			[]string{"aa"},
			3,
			[][]string{
				{"aa"},
			},
			nil,
		},
		{
			"large batch size",
			[]string{"aa", "bb", "cc", "dd", "ee", "ff"},
			100,
			[][]string{
				{"aa", "bb", "cc", "dd", "ee", "ff"},
			},
			nil,
		},
		{
			"zero element",
			[]string{},
			2,
			[][]string(nil),
			nil,
		},
		{
			"zero batch",
			[]string{"aa", "bb", "cc", "dd", "ee"},
			0,
			nil,
			fmt.Errorf("batch size must be positive. invalid size: 0"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches, err := createStringBatches(tt.elements, tt.size)
			assert.Equal(t, tt.expectedBatches, batches)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_copyStrings(t *testing.T) {
	tags := []string{"aa", "bb"}
	newTags := copyStrings(tags)
	assert.Equal(t, tags, newTags)
	assert.NotEqual(t, fmt.Sprintf("%p", tags), fmt.Sprintf("%p", newTags))
	assert.NotEqual(t, fmt.Sprintf("%p", &tags[0]), fmt.Sprintf("%p", &newTags[0]))
}
