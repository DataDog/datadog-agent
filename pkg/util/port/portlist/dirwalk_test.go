// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"go4.org/mem"
)

func TestWalkShallow(t *testing.T) {
	d := t.TempDir()

	t.Run("basics", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(d, "foo"), []byte("1"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "bar"), []byte("22"), 0400); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(d, "baz"), 0777); err != nil {
			t.Fatal(err)
		}

		var got []string
		if err := walkShallow(mem.S(d), func(name mem.RO, de os.DirEntry) error {
			var size int64
			if fi, err := de.Info(); err != nil {
				t.Errorf("Info stat error on %q: %v", de.Name(), err)
			} else if !fi.IsDir() {
				size = fi.Size()
			}
			got = append(got, fmt.Sprintf("%q %q dir=%v type=%d size=%v", name.StringCopy(), de.Name(), de.IsDir(), de.Type(), size))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sort.Strings(got)
		want := []string{
			`"bar" "bar" dir=false type=0 size=2`,
			`"baz" "baz" dir=true type=2147483648 size=0`,
			`"foo" "foo" dir=false type=0 size=1`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("mismatch:\n got %#q\nwant %#q", got, want)
		}
	})

	t.Run("err_not_exist", func(t *testing.T) {
		err := walkShallow(mem.S(filepath.Join(d, "not_exist")), func(name mem.RO, de os.DirEntry) error {
			return nil
		})
		if !os.IsNotExist(err) {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
