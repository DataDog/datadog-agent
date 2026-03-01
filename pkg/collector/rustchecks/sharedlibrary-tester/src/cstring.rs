use std::ffi::{CStr, c_char};

/// Helper function to safely convert C string to Rust string for printing
pub fn c_str_to_string(ptr: *mut c_char) -> String {
    if ptr.is_null() {
        "NULL".to_string()
    } else {
        unsafe { CStr::from_ptr(ptr) }
            .to_str()
            .unwrap_or("<invalid_utf8>")
            .to_string()
    }
}

/// Helper function to print C string array
pub fn c_str_array_to_vec(ptr: *mut *mut c_char) -> Vec<String> {
    if ptr.is_null() {
        return vec![];
    }
    
    let mut result = Vec::new();
    let mut current = ptr;
    
    
    unsafe {
        while !(*current).is_null() {
            result.push(c_str_to_string(*current));
            current = current.add(1);
        }
    }
    
    if result.is_empty() {
        vec![]
    } else {
        result
    }
}