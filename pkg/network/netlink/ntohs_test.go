package netlink

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNToHSU16(t *testing.T) {
	assert.Equal(t, uint16(80), NtohsU16(20480))
}
