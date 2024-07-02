// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"io"
	"io/fs"
	"os"

	"go4.org/mem"
)

func setOrCreateMap[K comparable, V any, T ~map[K]V](m *T, k K, v V) {
	if *m == nil {
		*m = make(map[K]V)
	}
	(*m)[k] = v
}

var osWalkShallow func(name mem.RO, fn walkFunc) error

// walkFunc is the callback type used with WalkShallow.
//
// The name and de are only valid for the duration of func's call
// and should not be retained.
type walkFunc func(name mem.RO, de fs.DirEntry) error

// dirWalkShallow reads the entries in the named directory and calls fn for each.
// It does not recurse into subdirectories.
//
// If fn returns an error, iteration stops and WalkShallow returns that value.
//
// On Linux, dirWalkShallow does not allocate, so long as certain methods on the
// walkFunc's DirEntry are not called which necessarily allocate.
func dirWalkShallow(dirName mem.RO, fn walkFunc) error {
	if f := osWalkShallow; f != nil {
		return f(dirName, fn)
	}
	of, err := os.Open(dirName.StringCopy())
	if err != nil {
		return err
	}
	defer of.Close()
	for {
		fis, err := of.ReadDir(100)
		for _, de := range fis {
			if err := fn(mem.S(de.Name()), de); err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
