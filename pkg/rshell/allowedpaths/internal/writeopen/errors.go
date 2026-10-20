// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package writeopen

import "errors"

// ErrSymlinkWriteTarget reports a symlink in a write target. Writes reject
// symlinks rather than following them so the target cannot change between
// resolution and open.
var ErrSymlinkWriteTarget = errors.New("symlinks are not supported as write targets")

// ErrNotRegularFile reports that a write target is not a regular file.
//
// It is returned both by the post-open fstat guard and by the open itself
// when the kernel refuses the open with ENXIO. ENXIO is not exclusively
// "FIFO opened O_WRONLY|O_NONBLOCK with no reader attached" — some device
// nodes report it too — but every ENXIO case here describes a non-regular
// target, which is the only thing rshell's write paths accept. Normalizing
// it keeps the message identical regardless of whether a reader happened to
// be attached at open time, and avoids leaking the raw platform errno text
// ("device not configured" on macOS, "no such device or address" on Linux).
var ErrNotRegularFile = errors.New("not a regular file")

// ErrIsDirectory reports that Unlink's target is a directory.
var ErrIsDirectory = errors.New("is a directory")

// ErrNotDirectory reports that a path passed to Unlink syntactically
// required its target to be a directory (a trailing separator, or a final
// "." or ".." component) but the resolved target is not one.
var ErrNotDirectory = errors.New("not a directory")
