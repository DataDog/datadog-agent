Real sysfs capture of an AMD Instinct MI350X SR-IOV virtual function (PCI ID
1002:75b0, subsystem 1002:75a0, gfx950, SPX/NPS1) on a DigitalOcean AMD
Developer Cloud droplet (`gpu-mi350x1-288gb`, Ubuntu 26.04, kernel 7.0.0), idle,
captured 2026-10-01 from /sys/class/drm/card1/device (attributes under
`device/` and `hwmon/hwmon0` under `hwmon/`) and
/sys/class/kfd/kfd/topology/nodes/1 (`kfd/`). `xcp_platform_devices` lists
/sys/devices/platform/amdgpu_xcp_* of the same host. The hardware serial and hive
identifiers are anonymized; other files preserve the captured sysfs content.
Empty or unreadable attributes were dropped.
