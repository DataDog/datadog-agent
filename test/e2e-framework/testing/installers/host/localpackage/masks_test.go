// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localpackage

import (
	"errors"
	"strings"
	"testing"
)

func TestPreexistingMasksAndInspectionFailuresNeverMutate(t *testing.T) {
	for _, state := range []string{"masked", "masked-runtime", "unexpected", "inspection-error"} {
		t.Run(state, func(t *testing.T) {
			calls := 0
			remote := Transport{Execute: func(cmd string) (string, error) {
				calls++
				if !strings.HasPrefix(cmd, "systemctl show") {
					t.Fatalf("mutated operator mask: %s", cmd)
				}
				if state == "inspection-error" {
					return "", errors.New("system bus unavailable")
				}
				return "LoadState=loaded\nUnitFileState=" + state, nil
			}}
			if _, err := acquireServiceMasks(remote, "test"); err == nil {
				t.Fatal("unsupported/preexisting policy accepted")
			}
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}
func TestReleaseRechecksIdentityAndNeverUnmasksForeignState(t *testing.T) {
	var commands []string
	remote := Transport{Execute: func(cmd string) (string, error) {
		commands = append(commands, cmd)
		switch {
		case strings.HasPrefix(cmd, "systemctl show"):
			return "LoadState=loaded\nUnitFileState=disabled", nil
		case strings.Contains(cmd, "ln -P"):
			return "12:345", nil
		case strings.Contains(cmd, "&& rm --"):
			for _, required := range []string{"stat -c %d:%i", "12:345", "readlink --", "/dev/null", "/run/e2ectl-masks-test/"} {
				if !strings.Contains(cmd, required) {
					t.Fatalf("missing ownership evidence %s: %s", required, cmd)
				}
			}
			return "", errors.New("administrator replaced mask")
		}
		if strings.Contains(cmd, "systemctl unmask") {
			t.Fatal("unmask-all used")
		}
		return "", nil
	}}
	release, err := acquireServiceMasks(remote, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = release(); err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatal(err)
	}
	all := strings.Join(commands, "\n")
	if strings.Contains(all, "sudo rmdir") {
		t.Fatal("removed repair evidence despite changed ownership")
	}
	if strings.Count(all, "systemctl daemon-reload") != 2 {
		t.Fatal(all)
	}
}
