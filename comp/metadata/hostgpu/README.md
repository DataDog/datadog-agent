# Host GPU Payload

This package populates some of the Agent-related fields in the `Resource Catalog` product in DataDog. More specifically the
`host_gpu_agent` table.

This legacy payload is disabled by default. Set `gpu.legacy_host_metadata` to `true` to enable it; `inventories_enabled` and `enable_metadata_collection` must also be enabled.

The payload is sent at least every 10min (see `inventories_max_interval` in the config). It is also added to flares as
`metadata/inventory/hostgpu.json`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `host_gpu_metadata` - **dict of string to JSON type**:
  - `devices` - **array**: list of gpu devices. Each element will have the following fields.
    - `gpu_index` - **int**:  the index of the gpu device as nvidia-smi or amd-smi reports it (e.g: 0, 1, ...); for MIG devices, the index NVML gives them within their parent; -1 when unknown (AMD GPUs whose amd-smi index can't be found in the KFD topology).
    - `gpu_vendor` - **string**: the GPU vendor, lowercased (`nvidia` or `amd`).
    - `gpu_device` - **string**:  Device is the commercial name of the device (e.g., Tesla V100).
    - `gpu_type` - **string**: The general type of the GPU device. (e.g: a100, t4, h100).
    - `gpu_slicing_mode` - **string**: The slicing mode of the GPU device. (e.g: mig, mig-parent, none).
    - `gpu_virtualization_mode` - **string**: The virtualization mode of the GPU device: `none`, `host_vgpu`,
      `passthrough`, `host_vsga`, `vgpu` or `unknown`. Empty for AMD devices, and for NVIDIA devices when NVML can't
      report it.
    - `gpu_driver_version` - **string**: The driver version.
    - `gpu_uuid` - **string**: Unique identifier of the device.
    - `gpu_architecture` - **string**: GPU device architecture (e.g: for nvidia, kepler, pascal, hopper).
    - `gpu_parent_uuid` - **string**: The UUID of the parent GPU device. Empty string if the device does not have a parent.
    - `gpu_compute_version` - **string**: GPU device compute capability, as `major.minor`. Always `"0.0"` for AMD devices.
    - `gpu_total_cores` - **int**: Number of total available cores on the device. Always `0` for AMD devices. For a
      MIG device (`mig`), this is its number of streaming multiprocessors, not cores. For a MIG-enabled parent
      (`mig-parent`), cores and memory are computed from MIG profile 0 times its instance count, so they can be lower
      than the full device.
    - `device_total_memory` - **uint64**: Total available memory on the device (in bytes).
    - `device_max_sm_clock_rate` - **uint32**: Device maximal Streaming Multiprocessor (SM) clock rate (in MHz).
    - `device_max_memory_clock_rate` - **uint32**:  Device maximal memory clock rate (in MHz).
    - `device_memory_bus_width` - **uint32**:  Device memory bus width (in bits).

## Example Payload

Here an example of an inventory payload:

```
{
    "host_gpu_metadata": {
        "devices": [
            {
                "gpu_index": 0,
                "gpu_vendor": "nvidia",
                "gpu_device": "NVIDIA A100-SXM4-80GB MIG 1g.10gb",
                "gpu_driver_version": "550.54.15",
                "gpu_uuid": "MIG-00000000-0000-0000-0000-000000000001",
                "gpu_architecture": "ampere",
                "gpu_compute_version": "8.0",
                "gpu_total_cores": 14,
                "gpu_type": "a100",
                "gpu_slicing_mode": "mig",
                "gpu_virtualization_mode": "passthrough",
                "gpu_parent_uuid": "GPU-12345678-1234-5678-1234-567812345678",
                "device_total_memory": 10200547328,
                "device_max_sm_clock_rate": 1410,
                "device_max_memory_clock_rate": 1593,
                "device_memory_bus_width": 5120
            },
            {
                "gpu_index": 1,
                "gpu_vendor": "nvidia",
                "gpu_device": "NVIDIA A100-SXM4-80GB",
                "gpu_driver_version": "550.54.15",
                "gpu_uuid": "GPU-12345678-1234-5678-1234-567812345678",
                "gpu_architecture": "ampere",
                "gpu_compute_version": "8.0",
                "gpu_total_cores": 6272,
                "gpu_type": "a100",
                "gpu_slicing_mode": "mig-parent",
                "gpu_virtualization_mode": "passthrough",
                "gpu_parent_uuid": "",
                "device_total_memory": 71403831296,
                "device_max_sm_clock_rate": 1410,
                "device_max_memory_clock_rate": 1593,
                "device_memory_bus_width": 5120
            }
        ]
    },
    "hostname": "my-host",
    "uuid": "00000000-0000-0000-0000-000000000001",
    "timestamp": 1631281754507358895
}
```
