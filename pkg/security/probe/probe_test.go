// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

func TestInvalidDiscarders(t *testing.T) {
	discarders := map[eval.Field][]interface{}{
		"open.filename": {
			"aaa",
			111,
			false,
		},
	}

	probe := NewProbe(nil)

	if !probe.isInvalidDiscarder("open.filename", dentryInvalidDiscarder) {
		t.Errorf("should be an invalid discarder")
	}
}
