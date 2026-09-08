// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};

pub(crate) struct TokenHandle(HANDLE);

unsafe impl Send for TokenHandle {}
unsafe impl Sync for TokenHandle {}

impl TokenHandle {
    pub(crate) fn new(handle: HANDLE) -> Self {
        Self(handle)
    }

    pub(crate) fn raw(&self) -> HANDLE {
        self.0
    }
}

impl Drop for TokenHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}
