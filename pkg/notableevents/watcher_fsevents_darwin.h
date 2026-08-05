// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#ifndef PKG_NOTABLEEVENTS_WATCHER_FSEVENTS_DARWIN_H
#define PKG_NOTABLEEVENTS_WATCHER_FSEVENTS_DARWIN_H

#include <stddef.h>
#include <stdint.h>

// dd_pkg_notableevents_fsevents_start creates and starts an FSEvents watcher
// and returns an opaque handle owned by the caller.
void *dd_pkg_notableevents_fsevents_start(
    const char *const *paths,
    size_t path_count,
    uintptr_t handle,
    char **error_message);

// dd_pkg_notableevents_fsevents_stop drains and releases an opaque watcher.
void dd_pkg_notableevents_fsevents_stop(void *watcher);

#endif
