// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	conf.addScalarOids([]string{""})
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
	conf.addColumnOids([]string{""})
	assert.ElementsMatch(t, []string{"1.1", "1.2", "1.3", "1.0"}, conf.ColumnOids)
}
