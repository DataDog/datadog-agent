use anyhow::{Ok, Result, bail};

use std::ffi::{CStr, CString, c_char};

/// Convert C-String pointer to Rust String
///
/// # Safety
/// `ptr` must be a valid, non-null, NUL-terminated C string for the duration of
/// this call.
pub unsafe fn to_rust_string(ptr: *const c_char) -> Result<String> {
    if ptr.is_null() {
        bail!("pointer to C-String is null, can't convert to Rust String")
    }

    let rust_str = unsafe { CStr::from_ptr(ptr) }.to_str()?;
    Ok(rust_str.to_string())
}

/// CStringGuard follows the RAII idiom to avoid freeing C-Strings manually.
/// It owns a CString and exposes a `*const c_char` pointer for FFI use.
pub struct CStringGuard {
    inner: CString,
}

impl CStringGuard {
    pub fn new(s: &str) -> Result<Self> {
        let inner = CString::new(s)?;
        Ok(Self { inner })
    }

    /// Returns a `*const c_char` pointer to the underlying C string.
    /// The pointer is valid for the lifetime of this guard.
    pub fn as_ptr(&self) -> *const c_char {
        self.inner.as_ptr()
    }
}

/// CStringArrayGuard owns a null-terminated array of CStrings for FFI use.
/// It exposes a `*const *const c_char` pointer suitable for the ACR callback ABI.
pub struct CStringArrayGuard {
    /// Owned CStrings -- kept alive so pointers remain valid
    _strings: Vec<CString>,
    /// Null-terminated array of pointers into `_strings`
    ptrs: Vec<*const c_char>,
}

impl CStringArrayGuard {
    pub fn new(arr: &[String]) -> Result<Self> {
        let strings: Vec<CString> = arr
            .iter()
            .map(|s| CString::new(s.as_str()))
            .collect::<std::result::Result<Vec<_>, _>>()?;

        let mut ptrs: Vec<*const c_char> = strings.iter().map(|s| s.as_ptr()).collect();
        ptrs.push(std::ptr::null()); // null-terminate

        Ok(Self {
            _strings: strings,
            ptrs,
        })
    }

    /// Returns a `*const *const c_char` pointer to the null-terminated array.
    /// The pointer is valid for the lifetime of this guard.
    pub fn as_ptr(&self) -> *const *const c_char {
        self.ptrs.as_ptr()
    }
}
