package checkconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_oidConfig_addScalarOids(t *testing.T) {
	conf := OidConfig{}

	assert.ElementsMatch(t, []string{}, conf.ScalarOids)

	conf.addScalarOids([]string{"1.1"})
	conf.addScalarOids([]string{"1.1"})
	conf.addScalarOids([]string{"1.2"})
	conf.addScalarOids([]string{"1.3"})
	conf.addScalarOids([]string{"1.0"})
	assert.ElementsMatch(t, []string{"1.1", "1.2", "1.3", "1.0"}, conf.ScalarOids)
}

func Test_oidConfig_addColumnOids(t *testing.T) {
	conf := OidConfig{}

	assert.ElementsMatch(t, []string{}, conf.ColumnOids)

	conf.addColumnOids([]string{"1.1"})
	conf.addColumnOids([]string{"1.1"})
	conf.addColumnOids([]string{"1.2"})
	conf.addColumnOids([]string{"1.3"})
	conf.addColumnOids([]string{"1.0"})
	assert.ElementsMatch(t, []string{"1.1", "1.2", "1.3", "1.0"}, conf.ColumnOids)
}
