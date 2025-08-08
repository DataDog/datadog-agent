use std::ffi::{c_char, CString};

pub fn to_cstring(string: &str) -> *mut c_char {
    CString::new(string).unwrap().into_raw()
}

pub fn to_cstring_array(vec: &[String]) -> *mut *mut c_char {
    let mut c_vec: Vec<*mut c_char> = vec.iter().map(|s| to_cstring(s)).collect();
    c_vec.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_vec.as_mut_ptr();
    std::mem::forget(c_vec); // prevent Rust from freeing the vector
    vec_ptr
}

pub fn free_cstring(ptr: *mut c_char) {
    unsafe { drop(CString::from_raw(ptr)) };
}

// should be used later to avoid memory leaks when rust types are converted to C types
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
