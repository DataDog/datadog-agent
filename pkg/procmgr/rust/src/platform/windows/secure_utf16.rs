// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;
use std::sync::atomic::{Ordering, compiler_fence};

#[derive(Clone)]
pub(crate) struct SecureUtf16String {
    data: Vec<u16>,
}

impl SecureUtf16String {
    pub(crate) fn from_utf16_units(units: &[u16]) -> Self {
        let mut data = Vec::with_capacity(units.len() + 1);
        data.extend_from_slice(units);
        data.push(0);
        Self { data }
    }

    #[cfg(test)]
    pub(crate) fn from_utf8(value: &str) -> Self {
        Self::from_utf16_units(&value.encode_utf16().collect::<Vec<_>>())
    }

    #[cfg(not(test))]
    pub(crate) fn is_empty(&self) -> bool {
        self.data.len() <= 1
    }

    pub(crate) fn as_ptr(&self) -> *const u16 {
        self.data.as_ptr()
    }
}

impl PartialEq for SecureUtf16String {
    fn eq(&self, other: &Self) -> bool {
        self.data == other.data
    }
}

impl Eq for SecureUtf16String {}

impl Drop for SecureUtf16String {
    fn drop(&mut self) {
        secure_zero_utf16(&mut self.data);
    }
}

pub(crate) fn secure_zero_utf16(data: &mut [u16]) {
    for unit in data.iter_mut() {
        unsafe {
            ptr::write_volatile(unit, 0);
        }
    }
    compiler_fence(Ordering::SeqCst);
}

#[cfg(not(test))]
pub(crate) fn secure_zero_bytes(data: &mut [u8]) {
    for byte in data.iter_mut() {
        unsafe {
            ptr::write_volatile(byte, 0);
        }
    }
    compiler_fence(Ordering::SeqCst);
}
