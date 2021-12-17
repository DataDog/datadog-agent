package tagset

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests just provide coverage of the default stubs. Other tests
// perform more thorough validation of the functionality.

func TestEmptyTags(t *testing.T) {
	require.Equal(t, 0, EmptyTags.Len())
}

func TestNewTags(t *testing.T) {
	tags := NewTags([]string{"a", "b", "a"})
	tags.validate(t)
}

func TestNewUniqueTags(t *testing.T) {
	tags := NewUniqueTags("a", "b")
	tags.validate(t)
}

func TestNewTagsFromMap(t *testing.T) {
	tags := NewTagsFromMap(map[string]struct{}{"a": {}, "b": {}})
	tags.validate(t)
}

func TestNewBuilder(t *testing.T) {
	b := NewBuilder(10)
	b.Add("a")
	b.Add("b")
	tags := b.Close()
	tags.validate(t)
}

func TestUnion(t *testing.T) {
	tags := Union(
		NewTags([]string{"a", "b", "c"}),
		NewTags([]string{"c", "d", "e"}),
	)
	tags.validate(t)
}

func TestDisjointUnion(t *testing.T) {
	tags := UnsafeDisjointUnion(
		NewTags([]string{"a", "b", "c"}),
		NewTags([]string{"d", "e"}),
	)
	tags.validate(t)
}
