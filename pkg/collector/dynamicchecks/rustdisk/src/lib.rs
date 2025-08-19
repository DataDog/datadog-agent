use serde::Serialize;
use std::alloc::{alloc, dealloc, Layout};
use std::ffi::{c_char, CString};
use std::os::raw::c_int;
use sysinfo::{Disks, System};

#[derive(Serialize)]
pub struct Metric {
    r#type: String,
    name: String,
    value: u64,
    tags: Vec<String>,
}

#[derive(Serialize)]
pub struct Payload {
    metrics: Vec<Metric>,
}

#[repr(C)]
pub struct Result {
    message: *mut c_char,
    len: c_int,
}

#[unsafe(no_mangle)]
pub extern "C" fn Run() -> *mut Result {
    let mut sys = System::new_all();
    sys.refresh_all();

    let mut value = 0;
    let mut tags = vec![];

    let disks = Disks::new_with_refreshed_list();

    for disk in &disks {
        if let Some(name) = disk.name().to_str() {
            if name.contains("sda") {
                value = disk.total_space(); // in bytes
                tags.push(format!("device:{}", name));
                break;
            }
        }
    }

    let payload = Payload {
        metrics: vec![Metric {
            r#type: "gauge".into(),
            name: "system.disk.total".into(),
            value,
            tags,
        }],
    };

    let json = serde_json::to_string(&payload).unwrap_or_default();
    let len = json.len() as c_int;
    let c_string = CString::new(json).unwrap();
    let raw_string = c_string.into_raw();

    unsafe {
        let layout = Layout::new::<Result>();
        let result_ptr = alloc(layout) as *mut Result;
        if result_ptr.is_null() {
            return std::ptr::null_mut();
        }
        (*result_ptr).message = raw_string;
        (*result_ptr).len = len;
        result_ptr
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn FreeResult(result: *mut Result) {
    if result.is_null() {
        return;
    }

    unsafe {
        // Convert raw C string back to CString and drop it to free memory
        let _ = CString::from_raw((*result).message);

        // Free the struct itself
        let layout = Layout::new::<Result>();
        dealloc(result as *mut u8, layout);
    }
}
