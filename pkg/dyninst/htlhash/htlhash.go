// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package htlhash computes the htl (head, tail, length) hash of a file.
//
// See https://opentelemetry.io/docs/specs/otel/profiles/mappings/#algorithm-for-processexecutablebuild_idhtlhash
// for more information.
package htlhash

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/sync"
)

// Hash is the htl (head, tail, length) hash of a file.
type Hash [16]byte

// Compute the hash of the first 4096 bytes of the file, the last 4096 bytes of
// the file, and the length of the file. This is a cheap to compute hash that is
// practically unique for the executable.
//
// Note that the file's position will be left in an unknown state after this.
//
// Derived from https://github.com/open-telemetry/opentelemetry-ebpf-profiler/blob/ec6ee459/libpf/fileid.go#L122-L128.
func Compute(f io.ReadSeeker) (Hash, error) {
	hasher := hasherPool.Get()
	defer hasherPool.Put(hasher)
	return hasher.hash(f)
}

type hasher struct {
	buf [4096]byte
	sha hash.Hash
}

var hasherPool = sync.NewTypedPool(func() *hasher {
	return &hasher{
		sha: sha256.New(),
	}
})

// String returns the hex-encoded hash.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

func (h *hasher) hash(f io.ReadSeeker) (_ Hash, retErr error) {
	maybeWrapErr := func(msg string, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("computeHtlHash: %s: %w", msg, err)
	}
	h.sha.Reset()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Hash{}, maybeWrapErr("failed to seek to original position", err)
	}
	{
		n, err := io.ReadFull(f, h.buf[:4096])
		if err != nil && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) {
			err = nil
		}
		if err != nil {
			return Hash{}, maybeWrapErr("failed to copy file to hash", err)
		}
		_, _ = h.sha.Write(h.buf[:n])
	}
	length, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return Hash{}, maybeWrapErr("failed to seek to end of file", err)
	}
	{
		tailBytes := min(length, 4096)
		if _, err := f.Seek(-tailBytes, io.SeekEnd); err != nil {
			return Hash{}, maybeWrapErr("failed to seek to end of file", err)
		}
		n, err := io.ReadFull(f, h.buf[:tailBytes])
		if err != nil && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) {
			err = nil
		}
		if err != nil {
			return Hash{}, maybeWrapErr("failed to copy file to hash", err)
		}
		_, _ = h.sha.Write(h.buf[:n])
	}
	{
		binary.BigEndian.PutUint64(h.buf[:8], uint64(length))
		_, _ = h.sha.Write(h.buf[:8])
	}
	sum := h.sha.Sum(h.buf[:0])
	var hash Hash
	copy(hash[:], sum[:16])
	return hash, nil
}
