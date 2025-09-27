use std::ffi::{c_char, CString};
use std::error::Error;

// TODO: raise errors in the conversion functions

pub fn to_cstring(string: &str) -> Result<*mut c_char, Box<dyn Error>> {
    let cstr = CString::new(string)?;

    Ok(cstr.into_raw())
}

pub fn to_cstring_array(vec: &[String]) -> Result<*mut *mut c_char, Box<dyn Error>> {
    let mut c_vec: Vec<*mut c_char> = vec.iter()
        .map(|s| to_cstring(s))
        .collect::<Result<Vec<_>, _>>()?;
    
    c_vec.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_vec.as_mut_ptr();
    std::mem::forget(c_vec); // prevent Rust runtime from freeing the vector
    
    Ok(vec_ptr)
}

pub fn free_cstring(ptr: *mut c_char) {
    if ptr.is_null() {
        return;
    }
    
    unsafe { drop(CString::from_raw(ptr)) };
}

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
