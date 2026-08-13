// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::mem;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};

pub(crate) struct WinHandle(HANDLE);

// SAFETY: Win32 kernel handles are safe to send across threads.
unsafe impl Send for WinHandle {}

impl WinHandle {
    pub(crate) fn new(handle: HANDLE) -> Self {
        Self(handle)
    }

    pub(crate) fn as_handle(&self) -> HANDLE {
        self.0
    }

    pub(crate) fn raw(self) -> HANDLE {
        let handle = self.0;
        mem::forget(self);
        handle
    }
}

impl Drop for WinHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}
