package tags

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	tests := []struct {
		tags     []string
		tagName  string
		expected string
	}{
		{
			[]string{"kube_pod:redis"},
			"kube_pod",
			"redis",
		},
		{
			[]string{"pod:redis"},
			"kube_pod",
			"",
		},
		{
			[]string{},
			"kube_pod",
			"",
		},
		{
			nil,
			"kube_pod",
			"",
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			require.Equal(t, tt.expected, Get(tt.tags, tt.tagName))
		})
	}
}
