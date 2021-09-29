// +build linux

package kernel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLockdown(t *testing.T) {
	mode := getLockdownMode(`none integrity [confidentiality]`)
	assert.Equal(t, Confidentiality, mode)

	mode = getLockdownMode(`none [integrity] confidentiality`)
	assert.Equal(t, Integrity, mode)

	mode = getLockdownMode(`[none] integrity confidentiality`)
	assert.Equal(t, None, mode)

	mode = getLockdownMode(`none integrity confidentiality`)
	assert.Equal(t, Unknown, mode)

	mode = getLockdownMode(`none integrity confidentiality [aaa]`)
	assert.Equal(t, Unknown, mode)
}
