// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package features

import (
	"errors"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
)

type fakeJitedSizer struct {
	size uint32
	err  error
}

func (f fakeJitedSizer) JitedSize() (uint32, error) { return f.size, f.err }

func TestRequireJIT(t *testing.T) {
	for _, tc := range []struct {
		name    string
		info    jitedSizer
		infoErr error
		wantErr bool
	}{
		{"jited", fakeJitedSizer{size: 64}, nil, false},
		{"interpreted or restricted", fakeJitedSizer{err: ebpf.ErrNotSupported}, nil, true},
		{"info unavailable", nil, errors.New("info failed"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := requireJIT(tc.info, tc.infoErr)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
