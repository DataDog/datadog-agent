// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package interp

import (
	"context"
	"io"
	"os"
)

func (r *Runner) builtinCat(ctx context.Context, args []string) exitStatus {
	if len(args) == 0 {
		args = []string{"-"}
	}
	for _, arg := range args {
		if err := r.catFile(ctx, arg); err != nil {
			r.errf("cat: %s: %v\n", arg, err)
			return exitStatus{code: 1}
		}
	}
	return exitStatus{}
}

func (r *Runner) catFile(ctx context.Context, path string) error {
	var rc io.ReadCloser
	if path == "-" {
		if r.stdin == nil {
			return nil
		}
		rc = io.NopCloser(r.stdin)
	} else {
		f, err := r.open(ctx, path, os.O_RDONLY, 0, false)
		if err != nil {
			return err
		}
		rc = f
	}
	defer rc.Close()
	_, err := io.Copy(r.stdout, rc)
	return err
}
