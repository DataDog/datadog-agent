package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoRegistryOrder(t *testing.T) {

	reg := NewInfoRegistry()
	info1 := NewCountInfo("1")
	info2 := NewCountInfo("2")
	info3 := NewCountInfo("3")

	reg.Register(info1)
	reg.Register(info2)
	reg.Register(info3)

	all := reg.All()

	assert.Equal(t, all[0].InfoKey(), "1")
	assert.Equal(t, all[1].InfoKey(), "2")
	assert.Equal(t, all[2].InfoKey(), "3")
}

func TestInfoRegistryReplace(t *testing.T) {

	reg := NewInfoRegistry()
	info1 := NewCountInfo("1")
	info1.Add(1)

	reg.Register(info1)

	all := reg.All()

	assert.Equal(t, "1", all[0].InfoKey())
	assert.Equal(t, "1", all[0].Info()[0])

	info2 := NewCountInfo("1")
	info2.Add(10)
	reg.Register(info2)

	all = reg.All()

	assert.Equal(t, "1", all[0].InfoKey())
	assert.Equal(t, "10", all[0].Info()[0])
}
