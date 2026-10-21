// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package usdt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func acquire(t *testing.T, r *SpecRegistry, spec Spec) (uint32, bool) {
	t.Helper()
	id, isNew, err := r.Acquire(spec)
	require.NoError(t, err)
	return id, isNew
}

func release(t *testing.T, r *SpecRegistry, id uint32) bool {
	t.Helper()
	freed, err := r.Release(id)
	require.NoError(t, err)
	return freed
}

func TestSpecRegistry(t *testing.T) {
	var r SpecRegistry
	a := specOf(regArg(1, x86RDI))
	b := specOf(constArg(1, 1))
	c := specOf(constArg(1, 0))

	idA, isNew := acquire(t, &r, a)
	assert.True(t, isNew)
	assert.EqualValues(t, 1, idA, "id 0 is reserved")

	id, isNew := acquire(t, &r, a)
	assert.False(t, isNew)
	assert.Equal(t, idA, id)

	idB, isNew := acquire(t, &r, b)
	assert.True(t, isNew)
	assert.EqualValues(t, 2, idB)
	assert.Equal(t, 2, r.Len())

	// a is still referenced once
	assert.False(t, release(t, &r, idA))
	assert.Equal(t, 2, r.Len())
	assert.True(t, release(t, &r, idA))
	assert.Equal(t, 1, r.Len())

	// the lowest free id is reused, for another spec
	idC, isNew := acquire(t, &r, c)
	assert.True(t, isNew)
	assert.Equal(t, idA, idC)

	// a gets a new id
	idA, isNew = acquire(t, &r, a)
	assert.True(t, isNew)
	assert.EqualValues(t, 3, idA)

	for _, id := range []uint32{idA, idB, idC} {
		assert.True(t, release(t, &r, id))
	}
	assert.Zero(t, r.Len())

	for _, id := range []uint32{0, idA, MaxSpecs, MaxSpecs + 1} {
		_, err := r.Release(id)
		assert.Error(t, err, "id %d", id)
	}
}

func TestSpecRegistryFull(t *testing.T) {
	var r SpecRegistry
	for i := 1; i < MaxSpecs; i++ {
		id, isNew := acquire(t, &r, specOf(constArg(8, int64(i))))
		assert.True(t, isNew)
		assert.EqualValues(t, i, id)
	}
	assert.Equal(t, MaxSpecs-1, r.Len())

	_, _, err := r.Acquire(specOf(constArg(8, 0)))
	assert.ErrorIs(t, err, ErrNoSpecID)

	// a spec in use can still be acquired
	id, isNew := acquire(t, &r, specOf(constArg(8, 42)))
	assert.False(t, isNew)
	assert.EqualValues(t, 42, id)

	assert.False(t, release(t, &r, 42))
	assert.True(t, release(t, &r, 42))
	id, isNew = acquire(t, &r, specOf(constArg(8, 0)))
	assert.True(t, isNew)
	assert.EqualValues(t, 42, id)
}
