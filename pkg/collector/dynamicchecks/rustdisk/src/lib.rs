extern crate flatbuffers;

use std::os::raw::c_int;
use sysinfo::{Disks, System};

// import the generated code
#[allow(dead_code, unused_imports)]
#[path = "../../payload_generated.rs"]
mod payload_generated;
pub use payload_generated::integrations::{Metric, MetricArgs, Payload};

use crate::payload_generated::integrations::PayloadArgs;

#[repr(C)]
pub struct Result {
    data: *const u8,
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
            value = disk.total_space(); // in bytes
            tags.push(format!("device:{}", name));
            break;
        }
    }

    let mut builder = flatbuffers::FlatBufferBuilder::with_capacity(1024);
    let metric_type = builder.create_string("gauge");
    let metric_name = builder.create_string("system.disk.total");
    let tag_strings: Vec<_> = tags.iter().map(|tag| builder.create_string(tag)).collect();
    let metric_tags = builder.create_vector(&tag_strings);

    let metric = Metric::create(
        &mut builder,
        &MetricArgs {
            type_: Some(metric_type),
            name: Some(metric_name),
            value: value as f64,
            tags: Some(metric_tags),
        },
    );

    let metrics = builder.create_vector(&[metric]);

    let payload = Payload::create(
        &mut builder,
        &PayloadArgs {
            metrics: Some(metrics),
        },
    );

    builder.finish(payload, None);
    let buf = builder.finished_data();

    let buf_vec = buf.to_vec();
    let result = Result {
        data: buf_vec.as_ptr(),
        len: buf_vec.len() as c_int,
    };

    // Keep the Vec alive by leaking it - Go will need to access this data
    std::mem::forget(buf_vec);

    Box::into_raw(Box::new(result))
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn FreeResult(result: *mut Result) {
    if result.is_null() {
        return;
    }

    unsafe {
        // Take ownership of the Result to properly drop the Vec<u8>
        let _ = Box::from_raw(result);
        // The Vec<u8> will be automatically dropped when the Box goes out of scope
    }
}
