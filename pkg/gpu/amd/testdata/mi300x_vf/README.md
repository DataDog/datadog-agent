Real sysfs capture of an AMD Instinct MI300X SR-IOV virtual function (PCI ID
1002:74b5, 0000:83:00.0) on an AMD Developer Cloud droplet (Ubuntu 24.04, kernel
6.8.0, amdgpu-dkms 6.19.14 from ROCm), idle, captured 2026-11-06. `device/`
holds attributes of the PCI device, including the gpu_metrics table (format
1.9, 50 attributes, 1218 bytes), and `hwmon/` attributes of its hwmon0
directory. The hwmon attributes were read a few milliseconds after
gpu_metrics. The device is alone in its XGMI hive.
