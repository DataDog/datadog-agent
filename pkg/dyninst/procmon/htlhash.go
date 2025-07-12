// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// computeHtlHash computes a hash of the first 4096 bytes of the file and the
// last 4096 bytes of the file, and the length of the file. This is a cheap to
// compute hash that is practically unique for the executable.
//
// Note that the file's position will be left in an unknown state after this.
//
// Derived from https://github.com/open-telemetry/opentelemetry-ebpf-profiler/blob/ec6ee459/libpf/fileid.go#L122-L128
// See also https://opentelemetry.io/docs/specs/otel/profiles/mappings/#algorithm-for-processexecutablebuild_idhtlhash
func computeHtlHash(f io.ReadSeeker) (_ string, retErr error) {
	maybeWrapErr := func(msg string, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("computeHtlHash: %s: %w", msg, err)
	}
	h := sha256.New()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", maybeWrapErr("failed to seek to original position", err)
	}
	if _, err := io.Copy(h, io.LimitReader(f, 4096)); err != nil {
		return "", maybeWrapErr("failed to copy file to hash", err)
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", maybeWrapErr("failed to seek to end of file", err)
	}
	tailBytes := min(size, 4096)
	if _, err := f.Seek(-tailBytes, io.SeekEnd); err != nil {
		return "", maybeWrapErr("failed to seek to end of file", err)
	}
	if _, err := io.Copy(h, io.LimitReader(f, tailBytes)); err != nil {
		return "", maybeWrapErr("failed to copy file to hash", err)
	}

	var lengthBuf [8]byte
	binary.BigEndian.PutUint64(lengthBuf[:], uint64(size))
	h.Write(lengthBuf[:])
	return hex.EncodeToString(h.Sum(nil)[:16]), nil
}
