# GPU resources

Builds and publishes static GPU inventory. `Snapshot` creates device fields; `NewPublisher` configures publication and `Publisher.Publish` sends updates.

`Snapshot` reads only facts cached in `safenvml.DeviceInfo` when the device is constructed (identity, PCI IDs, PCIe path, NUMA node, placement, host-link maxima); it makes no NVML or sysfs calls. Unknown values are omitted. Slot, tray, module and `platform.host_id` are only published with an identified chassis, like the matching metric tags. `host_link` carries maximum capabilities only; negotiated link state stays in metrics.
