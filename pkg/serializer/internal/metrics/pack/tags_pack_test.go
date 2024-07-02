package pack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagsPacker(t *testing.T) {
	p := &TagsPacker{}

	tags := make([]string, 0)
	consumer := func(s string) error {
		tags = append(tags, s)
		return nil
	}

	p.Pack([]string{"a", "b", "c"}, consumer)
	assert.Equal(t, []string{"a", "b", "c"}, tags)

	tags = tags[:0]
	p.Pack([]string{"a", "b", "c"}, consumer)
	assert.Equal(t, []string{"^0-3"}, tags)

	tags = tags[:0]
	p.Pack([]string{"a", "b", "d"}, consumer)
	assert.Equal(t, []string{"^0-2", "d"}, tags)

	tags = tags[:0]
	p.Pack([]string{"a", "b", "d"}, consumer)
	assert.Equal(t, []string{"^0-3"}, tags)

	tags = tags[:0]
	p.Pack([]string{"c", "b", "d"}, consumer)
	assert.Equal(t, []string{"c", "^1-2"}, tags)

	tags = tags[:0]
	p.Pack([]string{"d", "e", "f"}, consumer)
	assert.Equal(t, []string{"d", "e", "f"}, tags)

	tags = tags[:0]
	p.Pack([]string{"a", "e", "f"}, consumer)
	assert.Equal(t, []string{"a", "^1-2"}, tags)
}
