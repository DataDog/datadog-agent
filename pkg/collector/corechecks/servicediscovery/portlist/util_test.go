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
	"runtime"
	"sort"
	"testing"

	"go4.org/mem"
)

// replaceImpl replaces the value of target with val.
// The old value is restored when the test ends.
func replaceImpl[T any](t testing.TB, target *T, val T) {
	t.Helper()
	if target == nil {
		t.Fatalf("Replace: nil pointer")
		return
	}
	old := *target
	t.Cleanup(func() {
		*target = old
	})

	*target = val
}

func Test_setOrCreateMap(t *testing.T) {
	t.Run("unnamed", func(t *testing.T) {
		var m map[string]int
		setOrCreateMap(&m, "foo", 42)
		setOrCreateMap(&m, "bar", 1)
		setOrCreateMap(&m, "bar", 2)
		want := map[string]int{
			"foo": 42,
			"bar": 2,
		}
		if got := m; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v; want %v", got, want)
		}
	})
	t.Run("named", func(t *testing.T) {
		type M map[string]int
		var m M
		setOrCreateMap(&m, "foo", 1)
		setOrCreateMap(&m, "bar", 1)
		setOrCreateMap(&m, "bar", 2)
		want := M{
			"foo": 1,
			"bar": 2,
		}
		if got := m; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v; want %v", got, want)
		}
	})
}

func Test_dirWalkShallowOSSpecific(t *testing.T) {
	if osWalkShallow == nil {
		t.Skip("no OS-specific implementation")
	}
	testDirWalkShallow(t, false)
}

func Test_dirWalkShallowPortable(t *testing.T) {
	testDirWalkShallow(t, true)
}

func testDirWalkShallow(t *testing.T, portable bool) {
	if portable {
		replaceImpl(t, &osWalkShallow, nil)
	}
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
		if err := dirWalkShallow(mem.S(d), func(name mem.RO, de os.DirEntry) error {
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
		err := dirWalkShallow(mem.S(filepath.Join(d, "not_exist")), func(name mem.RO, de os.DirEntry) error {
			return nil
		})
		if !os.IsNotExist(err) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("allocs", func(t *testing.T) {
		allocs := int(testing.AllocsPerRun(1000, func() {
			if err := dirWalkShallow(mem.S(d), func(name mem.RO, de os.DirEntry) error { return nil }); err != nil {
				t.Fatal(err)
			}
		}))
		t.Logf("allocs = %v", allocs)
		if !portable && runtime.GOOS == "linux" && allocs != 0 {
			t.Errorf("unexpected allocs: got %v, want 0", allocs)
		}
	})
}
