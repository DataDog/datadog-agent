package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyTrace(t *testing.T) {
	tr := RandomTrace(3, 10)
	cp := CopyTrace(tr)
	assert.NotEqual(t, memaddr(tr), memaddr(cp))
}
