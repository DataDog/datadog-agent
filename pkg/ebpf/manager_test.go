// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build linux_bpf

package ebpf

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

type dummyModifier struct {
	mock.Mock
}

const dummyModifierName = "DummyModifier"

func (t *dummyModifier) String() string {
	// Do not mock this method for simplicity, to avoid having to define it always
	return dummyModifierName
}

func (t *dummyModifier) BeforeInit(m *manager.Manager, name names.ModuleName, opts *manager.Options) error {
	args := t.Called(m, name, opts)
	return args.Error(0)
}

func (t *dummyModifier) AfterInit(m *manager.Manager, name names.ModuleName, opts *manager.Options) error {
	args := t.Called(m, name, opts)
	return args.Error(0)
}

func (t *dummyModifier) BeforeStop(m *manager.Manager, name names.ModuleName, cleanupType manager.MapCleanupType) error {
	args := t.Called(m, name, cleanupType)
	return args.Error(0)
}

func (t *dummyModifier) AfterStop(m *manager.Manager, name names.ModuleName, cleanupType manager.MapCleanupType) error {
	args := t.Called(m, name, cleanupType)
	return args.Error(0)
}

func TestNewManagerWithDefault(t *testing.T) {
	type args struct {
		mgr       *manager.Manager
		modifiers []Modifier
	}
	// ensuring the lazy init of the defaultModifiers list
	_ = NewManagerWithDefault(nil, "ebpf", nil)
	tests := []struct {
		name                  string
		args                  args
		expectedModifierCount int
	}{
		{
			name:                  "with one custom modifier",
			args:                  args{mgr: nil, modifiers: []Modifier{&dummyModifier{}}},
			expectedModifierCount: len(defaultModifiers) + 1,
		},
		{
			name:                  "with empty modifiers list",
			args:                  args{mgr: nil, modifiers: []Modifier{}},
			expectedModifierCount: len(defaultModifiers),
		},
		{
			name:                  "passing nil as modifiers list",
			args:                  args{mgr: nil, modifiers: nil},
			expectedModifierCount: len(defaultModifiers),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := NewManagerWithDefault(tt.args.mgr, "ebpf", tt.args.modifiers...)
			assert.Equalf(t, tt.expectedModifierCount, len(target.EnabledModifiers), "Expected to have %v enabled modifiers ", tt.expectedModifierCount)
		})
	}
}

func TestManagerInitWithOptions(t *testing.T) {
	modifier := &dummyModifier{}
	modifier.On("BeforeInit", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	modifier.On("AfterInit", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mgr := NewManager(&manager.Manager{}, "test", modifier)
	require.NotNil(t, mgr)

	err := LoadCOREAsset("logdebug-test.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		err := mgr.InitWithOptions(buf, &opts)
		require.NoError(t, err)

		return nil
	})
	require.NoError(t, err)

	modifier.AssertExpectations(t)
}

func TestManagerStop(t *testing.T) {
	modifier := &dummyModifier{}
	modifier.On("BeforeStop", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	modifier.On("AfterStop", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mgr := NewManager(&manager.Manager{}, "test", modifier)
	require.NotNil(t, mgr)

	// The Stop call will fail because the manager is not initialized, but the modifiers should still be called
	_ = mgr.Stop(manager.CleanAll)

	modifier.AssertExpectations(t)
}
