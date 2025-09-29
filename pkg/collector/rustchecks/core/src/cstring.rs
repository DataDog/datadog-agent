use std::ffi::{c_char, CStr, CString};
use std::error::Error;

/// Convert C-String pointer to Rust String
pub fn to_rust_string(ptr: *mut c_char) -> Result<String, Box<dyn Error>> {
    let rust_str = unsafe { CStr::from_ptr(ptr) }.to_str()?;
    Ok(rust_str.to_string())
}

/// Convert Rust str to C-String pointer
pub fn to_cstring(string: &str) -> Result<*mut c_char, Box<dyn Error>> {
    let cstr = CString::new(string)?;

    Ok(cstr.into_raw())
}

/// Convert Rust vector of Strings to an array of C-String pointers
pub fn to_cstring_array(vec: &[String]) -> Result<*mut *mut c_char, Box<dyn Error>> {
    let mut c_vec: Vec<*mut c_char> = vec.iter()
        .map(|s| to_cstring(s))
        .collect::<Result<Vec<_>, _>>()?;
    
    c_vec.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_vec.as_mut_ptr();
    std::mem::forget(c_vec); // prevent Rust runtime from freeing the vector
    
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
    }
}
