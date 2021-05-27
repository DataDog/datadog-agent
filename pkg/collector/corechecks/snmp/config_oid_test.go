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

func Test_oidConfig_hasOids(t *testing.T) {
	tests := []struct {
		name            string
		scalarOids      []string
		columnOids      []string
		expectedHasOids bool
	}{
		{
			"has scalar oids",
			[]string{"1.2.3"},
			[]string{},
			true,
		},
		{
			"has scalar and column oids",
			[]string{"1.2.3"},
			[]string{"1.2.4"},
			true,
		},
		{
			"has no oids",
			[]string{},
			[]string{},
			false,
		},
		{
			"has no oids nil",
			nil,
			nil,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oc := &oidConfig{
				scalarOids: tt.scalarOids,
				columnOids: tt.columnOids,
			}
			hasOids := oc.hasOids()
			assert.Equal(t, tt.expectedHasOids, hasOids)
		})
	}
}
