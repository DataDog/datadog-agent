// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockConcreteWarning struct {
	id int
}

func (c mockConcreteWarning) Render() string {
	return "foo"
}

func TestWarnings(t *testing.T) {
	w = newWarnings()
	concreteWarning1 := mockConcreteWarning{id: 1}
	concreteWarning2 := mockConcreteWarning{id: 2}

	assert.Empty(t, GetWarnings())
	Raise("foo", concreteWarning1)
	assert.Equal(t, []Warning{concreteWarning1}, GetWarnings())
	Raise("bar", concreteWarning2)
	assert.ElementsMatch(t, []Warning{concreteWarning1, concreteWarning2}, GetWarnings())

	Remove("foo")
	assert.Equal(t, []Warning{concreteWarning2}, GetWarnings())
}
