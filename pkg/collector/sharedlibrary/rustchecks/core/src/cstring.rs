use anyhow::{Ok, Result, bail};

use std::ffi::{CStr, CString, c_char};

/// Convert C-String pointer to Rust String
pub fn to_rust_string(ptr: *const c_char) -> Result<String> {
    if ptr.is_null() {
        bail!("pointer to C-String is null, can't convert to Rust String")
    }

    let rust_str = unsafe { CStr::from_ptr(ptr) }.to_str()?;
    Ok(rust_str.to_string())
}

/// Convert Rust str to C-String pointer
pub fn to_cstring(string: &str) -> Result<*mut c_char> {
    let cstring = CString::new(string)?;
    Ok(cstring.into_raw())
}

/// Convert Rust vector of Strings to an array of C-String pointers
pub fn to_cstring_array(arr: &[String]) -> Result<*mut *mut c_char> {
    let mut c_arr: Vec<*mut c_char> = arr
        .iter()
        .map(|s| to_cstring(s))
        .collect::<Result<Vec<_>, _>>()?;

    c_arr.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_arr.as_mut_ptr();
    std::mem::forget(c_arr); // prevent Rust runtime from freeing the vector

    Ok(vec_ptr)
}

/// Free a C-String
pub fn free_cstring(ptr: *mut c_char) {
    if ptr.is_null() {
        return;
    }

    unsafe { drop(CString::from_raw(ptr)) };
}

/// Free an array of C-Strings
pub fn free_cstring_array(ptr: *mut *mut c_char) {
    if ptr.is_null() {
        return;
    }

    let mut current = ptr;
    unsafe {
        while !(*current).is_null() {
            drop(CString::from_raw(*current));
            current = current.add(1);
        }

        // Free the array itself by reconstructing the Vec
        let len = (current as usize - ptr as usize) / std::mem::size_of::<*mut c_char>();
        let capacity = len + 1; // +1 for the null terminator
        drop(Vec::from_raw_parts(ptr, len, capacity));
    }
}

/// CStringGuard follows the RAII idiom to avoid freeing C-Strings manually when it's necessary
pub struct CStringGuard {
    ptr: *mut c_char,
}

impl CStringGuard {
    pub fn new(s: &str) -> Result<Self> {
        let ptr = to_cstring(s)?;
        Ok(Self { ptr })
    }

    pub fn as_ptr(&self) -> *mut c_char {
        self.ptr
    }
}

impl Drop for CStringGuard {
    fn drop(&mut self) {
        free_cstring(self.ptr);
    }
}

pub struct CStringArrayGuard {
    ptr: *mut *mut c_char,
}

impl CStringArrayGuard {
    pub fn new(arr: &[String]) -> Result<Self> {
        let ptr = to_cstring_array(arr)?;
        Ok(Self { ptr })
    }

    pub fn as_ptr(&self) -> *mut *mut c_char {
        self.ptr
    }
}

impl Drop for CStringArrayGuard {
    fn drop(&mut self) {
        free_cstring_array(self.ptr);
    }
}
