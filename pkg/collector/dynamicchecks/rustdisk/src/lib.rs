#![feature(vec_into_raw_parts)]

extern crate flatbuffers;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::os::raw::c_int;
use std::path::Path;
use std::sync::{Mutex, OnceLock};
use sysinfo::{Disks, System};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Mount {
    #[serde(default)]
    pub host: String,
    #[serde(default)]
    pub share: String,
    #[serde(default)]
    pub user: String,
    #[serde(default)]
    pub password: String,
    #[serde(rename = "type", default)]
    pub mount_type: String,
    #[serde(default)]
    pub mountpoint: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiskInitConfig {
    #[serde(default)]
    pub device_global_exclude: Vec<String>,
    #[serde(default)]
    pub device_global_blacklist: Vec<String>,
    #[serde(default)]
    pub file_system_global_exclude: Vec<String>,
    #[serde(default)]
    pub file_system_global_blacklist: Vec<String>,
    #[serde(default)]
    pub mount_point_global_exclude: Vec<String>,
    #[serde(default)]
    pub mount_point_global_blacklist: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiskInstanceConfig {
    #[serde(default)]
    pub use_mount: bool,
    #[serde(default)]
    pub include_all_devices: bool,
    #[serde(default)]
    pub all_partitions: bool,
    #[serde(default)]
    pub min_disk_size: u64,
    #[serde(default)]
    pub tag_by_filesystem: bool,
    #[serde(default)]
    pub tag_by_label: bool,
    #[serde(default)]
    pub use_lsblk: bool,
    #[serde(default)]
    pub blkid_cache_file: String,
    #[serde(default)]
    pub service_check_rw: bool,
    #[serde(default)]
    pub create_mounts: Vec<Mount>,
    #[serde(default)]
    pub device_include: Vec<String>,
    #[serde(default)]
    pub device_whitelist: Vec<String>,
    #[serde(default)]
    pub device_exclude: Vec<String>,
    #[serde(default)]
    pub device_blacklist: Vec<String>,
    #[serde(default)]
    pub excluded_disks: Vec<String>,
    #[serde(default)]
    pub excluded_disk_re: String,
    #[serde(default)]
    pub file_system_include: Vec<String>,
    #[serde(default)]
    pub file_system_whitelist: Vec<String>,
    #[serde(default)]
    pub file_system_exclude: Vec<String>,
    #[serde(default)]
    pub file_system_blacklist: Vec<String>,
    #[serde(default)]
    pub excluded_filesystems: Vec<String>,
    #[serde(default)]
    pub mount_point_include: Vec<String>,
    #[serde(default)]
    pub mount_point_whitelist: Vec<String>,
    #[serde(default)]
    pub mount_point_exclude: Vec<String>,
    #[serde(default)]
    pub mount_point_blacklist: Vec<String>,
    #[serde(default)]
    pub excluded_mountpoint_re: String,
    #[serde(default)]
    pub device_tag_re: HashMap<String, String>,
    #[serde(default)]
    pub lowercase_device_tag: bool,
    #[serde(default)]
    pub timeout: u16,
    #[serde(default)]
    pub proc_mountinfo_path: String,
    #[serde(default)]
    pub resolve_root_device: bool,
}

// Configuration constants
static INIT_CONFIGURATION: OnceLock<Option<DiskInitConfig>> = OnceLock::new();
static INSTANCE_CONFIGURATIONS: OnceLock<Mutex<HashMap<String, DiskInstanceConfig>>> =
    OnceLock::new();

// import the generated code
#[allow(dead_code, unused_imports)]
#[path = "../../payload_generated.rs"]
mod payload_generated;
pub use payload_generated::integrations::{Configuration, Metric, MetricArgs, Payload};

use crate::payload_generated::integrations::PayloadArgs;

#[repr(C)]
pub struct Result {
    data: *mut u8,
    len: usize,
    cap: usize,
}

fn load_init_configuration() -> Option<DiskInitConfig> {
    let config_file = Path::new("/tmp/datadog-agent-checks/rustdisk/init.bin");
    if let Ok(config_data) = fs::read(config_file) {
        if let Ok(config) = flatbuffers::root::<Configuration>(&config_data) {
            if let Some(yaml_bytes) = config.value() {
                if let Ok(yaml_str) = std::str::from_utf8(yaml_bytes.bytes()) {
                    match serde_yaml::from_str::<DiskInitConfig>(yaml_str) {
                        Ok(parsed_config) => return Some(parsed_config),
                        Err(e) => {
                            eprintln!("Failed to parse YAML init configuration: {}", e);
                            return None;
                        }
                    }
                }
            }
        }
    }
    None
}

fn load_instance_configuration(id: &str) -> Option<DiskInstanceConfig> {
    let config_path = format!("/tmp/datadog-agent-checks/rustdisk/{}_instance.bin", id);
    let config_file = Path::new(&config_path);
    if let Ok(config_data) = fs::read(config_file) {
        if let Ok(config) = flatbuffers::root::<Configuration>(&config_data) {
            if let Some(yaml_bytes) = config.value() {
                if let Ok(yaml_str) = std::str::from_utf8(yaml_bytes.bytes()) {
                    match serde_yaml::from_str::<DiskInstanceConfig>(yaml_str) {
                        Ok(parsed_config) => return Some(parsed_config),
                        Err(e) => {
                            eprintln!("Failed to parse YAML instance configuration: {}", e);
                            return None;
                        }
                    }
                }
            }
        }
    }
    None
}

fn get_init_configuration() -> Option<DiskInitConfig> {
    INIT_CONFIGURATION
        .get_or_init(|| load_init_configuration())
        .clone()
}

fn get_instance_configuration(id: &str) -> Option<DiskInstanceConfig> {
    let configurations = INSTANCE_CONFIGURATIONS.get_or_init(|| Mutex::new(HashMap::new()));

    // Try to get from cache first
    {
        let cache = configurations.lock().unwrap();
        if let Some(config) = cache.get(id) {
            return Some(config.clone());
        }
    }

    // If not in cache, load from disk and cache it
    if let Some(config) = load_instance_configuration(id) {
        let mut cache = configurations.lock().unwrap();
        cache.insert(id.to_string(), config.clone());
        Some(config)
    } else {
        None
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn Allocate() -> *mut Result {
    let v = Vec::<u8>::new();
    let (data, len, cap) = v.into_raw_parts();

    Box::into_raw(Box::new(Result { data, len, cap }))
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn Run(id: *const std::os::raw::c_char, result: *mut Result) {
    if id.is_null() {
        eprintln!("Error: id parameter is null");
        return;
    }

    let id_str = unsafe { std::ffi::CStr::from_ptr(id).to_str().unwrap_or("") };

    if result.is_null() {
        eprintln!("Error: result parameter is null");
        return;
    }

    // Get init configuration (loaded once automatically)
    let init_config = get_init_configuration();
    if let Some(_cfg) = init_config {
        eprintln!("Init configuration loaded");
    }

    // Get instance configuration for this specific instance
    let instance_config = get_instance_configuration(id_str);
    if let Some(ref cfg) = instance_config {
        eprintln!(
            "Instance configuration loaded for {}: min_disk_size: {}, all_partitions: {}",
            id_str, cfg.min_disk_size, cfg.all_partitions
        );
    }

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

    // Store the new data in the result
    let (data, len, cap) = buf_vec.into_raw_parts();
    unsafe {
        (*result).data = data;
        (*result).len = len;
        (*result).cap = cap;
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn FreeResult(result: *mut Result) {
    if result.is_null() {
        return;
    }

    unsafe {
        // Reconstruct and drop the Vec to free the underlying data
        let result_box = Box::from_raw(result);
        if !result_box.data.is_null() {
            let _vec = Vec::from_raw_parts(result_box.data, result_box.len, result_box.cap);
            // Vec will be dropped automatically, freeing the memory
        }
        // result_box is also dropped automatically
    }
}
