package enrichment

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMapEtherType(t *testing.T) {
	assert.Equal(t, "", MapEtherType(0))
	assert.Equal(t, "", MapEtherType(0x8888))
	assert.Equal(t, "IPv4", MapEtherType(0x0800))
	assert.Equal(t, "IPv6", MapEtherType(0x86DD))
}
