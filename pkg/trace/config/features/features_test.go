package features

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeatures(t *testing.T) {
	Set("a, b ,c")
	assert.True(t, Has("a"))
	assert.True(t, Has("b"))
	assert.True(t, Has("c"))
	assert.ElementsMatch(t, All(), []string{"a", "b", "c"})
}
