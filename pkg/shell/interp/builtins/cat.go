// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"io"
	"os"
)

func builtinCat(ctx context.Context, call *CallContext, args []string) Result {
	if len(args) == 0 {
		args = []string{"-"}
	}
	for _, arg := range args {
		if err := catFile(ctx, call, arg); err != nil {
			call.Errf("cat: %s: %v\n", arg, err)
			return Result{Code: 1}
		}
	}
	return Result{}
}

func catFile(ctx context.Context, call *CallContext, path string) error {
	var rc io.ReadCloser
	if path == "-" {
		if call.Stdin == nil {
			return nil
		}
		rc = io.NopCloser(call.Stdin)
	} else {
		f, err := call.OpenFile(ctx, path, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		rc = f
	}
	defer rc.Close()
	_, err := io.Copy(call.Stdout, rc)
	return err
}
