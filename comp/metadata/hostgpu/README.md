# Inventory Host Payload

This package populates some of the Agent-related fields in the `Resource Catalog` product in DataDog. More specifically the
`host_gpu_agent` table.

This is enabled by default but can be turned off using `inventories_gpu_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `host_gpu_metadata` - **dict of string to JSON type**:
  - `devices` - **array**: list of gpu devices. Each element will have the following fields.
    - `gpu_index` - **int**:  the internal index of the gpu device (e.g: 0, 1, ...).
    - `gpu_vendor` - **string**: the GPU vendor. (e.g: nvidia, amd, intel. Currently only nvidia devices are supported.
    - `gpu_device` - **string**:  Device is the commercial name of the device (e.g., Tesla V100).
    - `gpu_type` - **string**: The general type of the GPU device. (e.g: a100, t4, h100).
    - `gpu_slicing_mode` - **string**: The slicing mode of the GPU device. (e.g: mig, mig-parent, none).
    - `gpu_virtualization_mode` - **string**: The virtualization mode of the GPU device. (e.g: vgpu, passthrough).
    - `gpu_driver_version` - **string**: The driver version.
    - `gpu_uuid` - **string**: Unique identifier of the device.
    - `gpu_architecture` - **string**: GPU device architecture (e.g: for nvidia, kepler, pascal, hopper.
    - `gpu_parent_uuid` - **string**: The UUID of the parent GPU device. Empty string if the device does not have a parent.
    - `gpu_compute_version` - **string**: GPU device compute capability.
    - `gpu_total_cores` - **int**: Number of total available cores on the device.
    - `device_total_memory` - **uint64**: Total available memory on the device (in bytes).
    - `device_max_sm_clock_rate` - **uint32**: Device maximal Streaming Multiprocessor (SM) clock rate (in MHz).
    - `device_max_memory_clock_rate` - **string**:  Device maximal memory clock rate (in MHz).
    - `device_memory_bus_width` - **string**:  Device memory bus width (in bits).

## Example Payload

Here an example of an inventory payload:

```
{
    "host_gpu_metadata": {
        "devices": [
            {
                "gpu_index": 0,
                "gpu_vendor": "nvidia",
                "gpu_device": "Tesla V100",
                "gpu_driver_version": "460.32.03",
                "gpu_uuid": "GPU-12345678-1234-5678-1234-567812345678",
                "gpu_architecture": "volta",
                "gpu_compute_version": "7.0",
                "gpu_total_cores": 4010,
                "gpu_type": "a100",
                "gpu_slicing_mode": "mig",
                "gpu_virtualization_mode": "vgpu",
                "gpu_parent_uuid": "GPU-12345678-1234-5678-1234-567812345678",
                "device_total_memory": 16384,
                "device_max_sm_clock_rate": 1530,
                "device_max_memory_clock_rate": "877",
                "device_memory_bus_width": "4096"
            },
            {
                "gpu_index": 1,
                "gpu_vendor": "nvidia",
                "gpu_device": "Tesla T4",
                "gpu_driver_version": "460.32.03",
                "gpu_uuid": "GPU-87654321-4321-8765-4321-876543218765",
                "gpu_architecture": "turing",
                "gpu_compute_version": "7.5",
                "gpu_total_cores": 2040,
                "gpu_type": "t4",
                "gpu_slicing_mode": "none",
                "gpu_virtualization_mode": "passthrough",
                "gpu_parent_uuid": "",
                "device_total_memory": 16384,
                "device_max_sm_clock_rate": 1590,
                "device_max_memory_clock_rate": "6251",
                "device_memory_bus_width": "256"
            }
        ]
    },
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
