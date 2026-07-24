// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#include "watcher_fsevents_darwin.h"

#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

extern void goPkgNotableEventsFSEventsCallback(uintptr_t handle, const char *path, uint32_t flags);

typedef struct {
    FSEventStreamRef stream;
    dispatch_queue_t queue;
} dd_pkg_notableevents_fsevents_watcher;

// dd_pkg_notableevents_fsevents_callback forwards native filesystem events to
// the Go watcher associated with this stream.
static void dd_pkg_notableevents_fsevents_callback(
    ConstFSEventStreamRef stream,
    void *context,
    size_t event_count,
    void *event_paths,
    const FSEventStreamEventFlags event_flags[],
    const FSEventStreamEventId event_ids[]) {
    (void)stream;
    (void)event_ids;

    char **paths = event_paths;
    uintptr_t handle = (uintptr_t)context;
    for (size_t i = 0; i < event_count; i++) {
        goPkgNotableEventsFSEventsCallback(handle, paths[i], event_flags[i]);
    }
}

// dd_pkg_notableevents_fsevents_queue_barrier provides a synchronization point
// that guarantees queued callbacks have completed before stream teardown.
static void dd_pkg_notableevents_fsevents_queue_barrier(void *context) {
    (void)context;
}

// dd_pkg_notableevents_set_error returns an allocated error message to the Go
// caller when native watcher setup fails.
static void dd_pkg_notableevents_set_error(char **error_message, const char *message) {
    if (error_message != NULL) {
        *error_message = strdup(message);
    }
}

// dd_pkg_notableevents_fsevents_start creates and starts a serial FSEvents
// stream for the supplied DiagnosticReports directories.
void *dd_pkg_notableevents_fsevents_start(
    const char *const *paths,
    size_t path_count,
    uintptr_t handle,
    char **error_message) {
    if (path_count == 0) {
        dd_pkg_notableevents_set_error(error_message, "cannot start FSEvents without paths");
        return NULL;
    }

    CFMutableArrayRef watched_paths = CFArrayCreateMutable(
        kCFAllocatorDefault,
        (CFIndex)path_count,
        &kCFTypeArrayCallBacks);
    if (watched_paths == NULL) {
        dd_pkg_notableevents_set_error(error_message, "failed to allocate FSEvents path array");
        return NULL;
    }

    for (size_t i = 0; i < path_count; i++) {
        CFStringRef path = CFStringCreateWithCString(
            kCFAllocatorDefault,
            paths[i],
            kCFStringEncodingUTF8);
        if (path == NULL) {
            CFRelease(watched_paths);
            dd_pkg_notableevents_set_error(error_message, "failed to convert an FSEvents path");
            return NULL;
        }
        CFArrayAppendValue(watched_paths, path);
        CFRelease(path);
    }

    dd_pkg_notableevents_fsevents_watcher *watcher = calloc(1, sizeof(*watcher));
    if (watcher == NULL) {
        CFRelease(watched_paths);
        dd_pkg_notableevents_set_error(error_message, "failed to allocate FSEvents watcher");
        return NULL;
    }

    FSEventStreamContext stream_context = {
        .version = 0,
        .info = (void *)handle,
        .retain = NULL,
        .release = NULL,
        .copyDescription = NULL,
    };
    watcher->stream = FSEventStreamCreate(
        kCFAllocatorDefault,
        dd_pkg_notableevents_fsevents_callback,
        &stream_context,
        watched_paths,
        kFSEventStreamEventIdSinceNow,
        0.5,
        kFSEventStreamCreateFlagWatchRoot);
    CFRelease(watched_paths);
    if (watcher->stream == NULL) {
        free(watcher);
        dd_pkg_notableevents_set_error(error_message, "failed to create FSEvents stream");
        return NULL;
    }

    watcher->queue = dispatch_queue_create("com.datadoghq.agent.pkg.notableevents.fsevents", DISPATCH_QUEUE_SERIAL);
    if (watcher->queue == NULL) {
        FSEventStreamRelease(watcher->stream);
        free(watcher);
        dd_pkg_notableevents_set_error(error_message, "failed to create FSEvents dispatch queue");
        return NULL;
    }

    FSEventStreamSetDispatchQueue(watcher->stream, watcher->queue);
    if (!FSEventStreamStart(watcher->stream)) {
        FSEventStreamInvalidate(watcher->stream);
        FSEventStreamRelease(watcher->stream);
        dispatch_release(watcher->queue);
        free(watcher);
        dd_pkg_notableevents_set_error(error_message, "failed to start FSEvents stream");
        return NULL;
    }

    return watcher;
}

// dd_pkg_notableevents_fsevents_stop drains callbacks and releases all native
// resources owned by an FSEvents watcher.
void dd_pkg_notableevents_fsevents_stop(void *raw_watcher) {
    if (raw_watcher == NULL) {
        return;
    }

    dd_pkg_notableevents_fsevents_watcher *watcher = raw_watcher;
    FSEventStreamStop(watcher->stream);
    FSEventStreamInvalidate(watcher->stream);
    dispatch_sync_f(watcher->queue, NULL, dd_pkg_notableevents_fsevents_queue_barrier);
    FSEventStreamRelease(watcher->stream);
    dispatch_release(watcher->queue);
    free(watcher);
}
