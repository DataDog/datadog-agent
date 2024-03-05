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
)

// PrintkPatcherModifier adds an InstructionPatcher to the manager that removes the newline character from log_debug calls if needed
type dummyModifier struct {
}

const dummyModifierName = "DummyModifier"

func (t *dummyModifier) String() string {
	return dummyModifierName
}

// BeforeInit adds the patchPrintkNewline function to the manager
func (t *dummyModifier) BeforeInit(_ *manager.Manager, _ *manager.Options) error { return nil }

// AfterInit is a no-op for this modifier
func (t *dummyModifier) AfterInit(_ *manager.Manager, _ *manager.Options) error {
	return nil
}

func TestNewManagerWithDefault(t *testing.T) {
	type args struct {
		mgr       *manager.Manager
		modifiers []Modifier
	}
	// ensuring the lazy init of the defaultModifiers list
	_ = NewManagerWithDefault(nil, nil)
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
			target := NewManagerWithDefault(tt.args.mgr, tt.args.modifiers...)
			assert.Equalf(t, tt.expectedModifierCount, len(target.EnabledModifiers), "Expected to have %v enabled modifiers ", tt.expectedModifierCount)
		})
	}
}
