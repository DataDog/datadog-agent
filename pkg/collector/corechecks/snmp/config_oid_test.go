package snmp

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_oidConfig_addScalarOids(t *testing.T) {
	conf := oidConfig{}

	assert.ElementsMatch(t, []string{}, conf.scalarOids)

	conf.addScalarOids([]string{"1.1"})
	conf.addScalarOids([]string{"1.1"})
	conf.addScalarOids([]string{"1.2"})
	conf.addScalarOids([]string{"1.3"})
	conf.addScalarOids([]string{"1.0"})
	assert.ElementsMatch(t, []string{"1.1", "1.2", "1.3", "1.0"}, conf.scalarOids)
}

func Test_oidConfig_addColumnOids(t *testing.T) {
	conf := oidConfig{}

	assert.ElementsMatch(t, []string{}, conf.columnOids)

	conf.addColumnOids([]string{"1.1"})
	conf.addColumnOids([]string{"1.1"})
	conf.addColumnOids([]string{"1.2"})
	conf.addColumnOids([]string{"1.3"})
	conf.addColumnOids([]string{"1.0"})
	assert.ElementsMatch(t, []string{"1.1", "1.2", "1.3", "1.0"}, conf.columnOids)
}
