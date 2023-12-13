// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package netstat

import (
	"testing"
)

func TestGet(t *testing.T) {
	nt, err := Get()
	if err == ErrNotImplemented {
		t.Skip("TODO: not implemented")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range nt.Entries {
		t.Logf("Entry: %+v", e)
	}
}
