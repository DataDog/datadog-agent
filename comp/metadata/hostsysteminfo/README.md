# Host System Info Payload

This package populates the host system information fields in the `End User Device Monitoring` product in Datadog.

This is enabled only for the `end_user_device` infrastructure mode.

The payload is sent every 1 hour (see `inventories_max_interval` in the config) or whenever it's updated with at most 1 update every 1 hour (see `inventories_min_interval`).

# Content

The payload contains physical system identification attributes collected from the host system, including manufacturer details, model information, serial numbers, and chassis type. This information is useful for asset management, hardware inventory tracking, and fleet management in end-user device monitoring scenarios.

## System Information Collection

The system information is collected using platform-specific APIs:
- **Windows**: WMI queries (`Win32_ComputerSystem`, `Win32_BIOS`, `Win32_SystemEnclosure`)
- **MacOS**: IOKit queries (`IOPlatformExpertDevice`, `product`)
- **Linux/Unix**: Will not run as it is currently not implemented

Collection includes:
- Manufacturer name (for example, Dell, Lenovo, HP, Amazon EC2)
- Model number and name
- Serial number
- System SKU/Identifier
- Chassis type (Desktop, Laptop, Virtual Machine, Other)
- Computer name (macOS only)

## Configuration

System info metadata collection can be controlled using:
- `infrastructure_mode: end_user_device` - Required to enable this feature

# Format

The payload is a JSON dict with the following fields:

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created (Unix nanoseconds).
- `host_system_info` - **dict of string to JSON type**:
  - `manufacturer` - **string**: The company brand name under which the device is marketed.
  - `model_number` - **string**: Company's specific model number of device.
  - `serial_number` - **string**: The serial number assigned from the company and is accessible on the exterior of the device.
  - `model_name` - **string**: The model name of the current device.
  - `chassis_type` - **string**: The chassis type of the current device. One of: "Desktop", "Laptop", "Virtual Machine", or "Other".
  - `identifier` - **string**: the system SKU number or other unique identifier.
  - `computer_name` - **string**: the user-assigned device name (macOS only, e.g. from System Settings > Sharing). Empty on other platforms.

## Virtual Machine Detection

The payload includes special logic to detect virtual machines:
- Hyper-V and Azure VMs are detected via the model name "Virtual Machine"
- AWS EC2 instances are detected via the manufacturer "Amazon EC2"
- When detected, the `chassis_type` is set to "Virtual Machine"

## Example Payload

Here is an example of a host system info payload for a physical laptop:

```json
{
    "hostname": "LAPTOP-123456",
    "timestamp": 1767996703894578400,
    "host_system_info_metadata": {
        "manufacturer": "LENOVO",
        "model_number": "ABC123",
        "serial_number": "DEF456",
        "model_name": "ThinkPad T14s Gen 5",
        "chassis_type": "Laptop",
        "identifier": "LENOVO_MT_21LS_BU_Think_FM_ThinkPad T14s Gen 5",
        "computer_name": ""
    },
    "uuid": "1234-5678-abcd-efgh"
}
```

Here is an example for a virtual machine:

```json
{
    "hostname": "WIN-VM",
    "timestamp": 1767998956607294100,
    "host_system_info_metadata": {
        "manufacturer": "Microsoft Corporation",
        "model_number": "Virtual Machine",
        "serial_number": "XYZ789",
        "model_name": "Virtual Machine",
        "chassis_type": "Virtual Machine",
        "identifier": "None",
        "computer_name": ""
    },
    "uuid": "abcd-1234-efgh-5678"
}
```

Here is an example for a macOS laptop:

```json
{
    "hostname": "MacBook-Pro",
    "timestamp": 1767998956607294100,
    "host_system_info_metadata": {
        "manufacturer": "Apple Inc.",
        "model_number": "A2779LL/A",
        "serial_number": "FVFXC1ABCD",
        "model_name": "MacBook Pro",
        "chassis_type": "Laptop",
        "identifier": "MacBookPro18,1",
        "computer_name": "Jane's MacBook Pro"
    },
    "uuid": "1234-abcd-5678-efgh"
}
```
