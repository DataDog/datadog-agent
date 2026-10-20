// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package stat implements the stat filesystem-status builtin.
//
// Only GNU stat's -f/--file-system mode is supported. Operands are resolved
// through CallContext.FileSystemStat and remain subject to AllowedPaths.
package stat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/flagparser"
)

// Cmd is the stat builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "stat",
	Description: "report file system status",
	MakeFlags:   makeFlags,
}

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	fileSystem := flagparser.RegisterNoArgBool(fs, "file-system", "f", "display file system status")
	help := flagparser.RegisterNoArgBool(fs, "help", "h", "print usage and exit")

	return func(ctx context.Context, callCtx *builtins.CallContext, paths []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if !*fileSystem {
			callCtx.Errf("stat: file status mode is not supported; use 'stat -f FILE...'\n")
			return builtins.Result{Code: 1}
		}

		if len(paths) == 0 {
			callCtx.Errf("stat: missing operand\n")
			callCtx.Errf("Try 'stat --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		if callCtx.FileSystemStat == nil {
			callCtx.Errf("stat: file system status capability not available\n")
			return builtins.Result{Code: 1}
		}

		failed := false
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return builtins.Result{Code: 1}
			}

			if path == "-" {
				callCtx.Errf("stat: using '-' to denote standard input does not work in file system mode\n")
				failed = true
				continue
			}

			info, err := callCtx.FileSystemStat(ctx, path)
			if err != nil {
				callCtx.Errf("stat: cannot read file system information for %s: %s\n", strconv.Quote(path), portableError(callCtx, err))
				failed = true
				continue
			}

			writeFileSystemInfo(callCtx, path, info)
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

func writeFileSystemInfo(callCtx *builtins.CallContext, path string, info builtins.FileSystemInfo) {
	id := unavailableOr(info.ID, info.IDAvailable, "-", 16)
	nameMax := unavailableOr(info.NameMax, info.NameMaxAvailable, "?", 10)
	files := unavailableOr(info.Files, info.FilesAvailable, "-", 10)
	filesFree := unavailableOr(info.FilesFree, info.FilesAvailable, "-", 10)

	typeName := info.TypeName
	if typeName == "" {
		if info.TypeIDAvailable {
			typeName = fmt.Sprintf("UNKNOWN (0x%x)", info.TypeID)
		} else {
			typeName = "?"
		}
	}

	callCtx.Outf("  File: %s\n", strconv.Quote(path))
	callCtx.Outf("    ID: %-8s Namelen: %-7s Type: %s\n", id, nameMax, typeName)
	callCtx.Outf("Block size: %-10d Fundamental block size: %d\n", info.IOBlockSize, info.FundamentalBlockSize)
	callCtx.Outf("Blocks: Total: %-10d Free: %-10d Available: %d\n", info.Blocks, info.BlocksFree, info.BlocksAvailable)
	callCtx.Outf("Inodes: Total: %-10s Free: %s\n", files, filesFree)
}

func unavailableOr(value uint64, available bool, unavailable string, base int) string {
	if !available {
		return unavailable
	}
	return strconv.FormatUint(value, base)
}

func portableError(callCtx *builtins.CallContext, err error) string {
	// The diagnostic already includes a safely quoted operand. Strip the
	// operation and path carried by os.PathError so control characters cannot
	// be reintroduced by the error string and the operand is not printed twice.
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		err = pathErr.Err
	}
	if callCtx.PortableErr != nil {
		return callCtx.PortableErr(err)
	}
	return err.Error()
}

func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: stat -f FILE...\n")
	callCtx.Out("Display file system status for each FILE.\n")
	callCtx.Out("Ordinary file status mode is not supported.\n\n")

	// RegisterNoArgBool uses an unforgeable NUL sentinel for bare flags.
	// Clear it while rendering defaults so help output contains no NUL byte.
	var saved []*builtins.Flag
	fs.VisitAll(func(flag *builtins.Flag) {
		if flag.NoOptDefVal == flagparser.NoArgSentinel {
			saved = append(saved, flag)
			flag.NoOptDefVal = ""
		}
	})
	defer func() {
		for _, flag := range saved {
			flag.NoOptDefVal = flagparser.NoArgSentinel
		}
	}()

	fs.SetOutput(callCtx.Stdout)
	fs.PrintDefaults()
}
