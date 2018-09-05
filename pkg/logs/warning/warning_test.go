// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package warning

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

func TestRaise(t *testing.T) {
	w = newWarnings()
	concreteWarning1 := mockConcreteWarning{id: 1}
	concreteWarning2 := mockConcreteWarning{id: 2}

	assert.Empty(t, w.raised)
	Raise("foo", concreteWarning1)
	assert.Equal(t, concreteWarning1, w.raised["foo"])
	Raise("bar", concreteWarning2)
	assert.Equal(t, concreteWarning2, w.raised["bar"])
}

func TestGet(t *testing.T) {
	w = newWarnings()
	assert.Empty(t, Get())

	concreteWarning1 := mockConcreteWarning{id: 1}
	concreteWarning2 := mockConcreteWarning{id: 2}
	w.raised["foo"] = concreteWarning1
	assert.Equal(t, []Warning{concreteWarning1}, Get())
	w.raised["bar"] = concreteWarning2
	assert.Equal(t, []Warning{concreteWarning1, concreteWarning2}, Get())
}

func TestRemove(t *testing.T) {
	w = newWarnings()
	assert.Empty(t, w.raised)

	concreteWarning1 := mockConcreteWarning{id: 1}
	concreteWarning2 := mockConcreteWarning{id: 2}
	w.raised["foo"] = concreteWarning1
	w.raised["bar"] = concreteWarning2
	assert.Equal(t, 2, len(w.raised))
	Remove("foo")
	assert.Equal(t, 1, len(w.raised))
	assert.Equal(t, []Warning{concreteWarning2}, Get())
}
