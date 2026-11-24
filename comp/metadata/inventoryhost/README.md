# Inventory Host Payload

This package populates some of the Agent-related fields in the `inventories` product in DataDog. More specifically the
`host_agent` table.

This is enabled by default but can be turned off using the `inventories_enabled` or `enable_metadata_collection`
config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every minute (see `inventories_min_interval`).

# Content

The `Refresh` method from the component triggers a new payload to be sent, while still respecting the minimal interval
between two updates.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `host_metadata` - **dict of string to JSON type**:
  - `cpu_cores` - **int**: the number of core for the host.
  - `cpu_logical_processors` - **int**:  the number of logical cores for the host.
  - `cpu_vendor` - **string**: the CPU vendor.
  - `cpu_model` - **string**:  the CPU model.
  - `cpu_model_id` - **string**: the CPU model ID.
  - `cpu_family` - **string**: the CPU family.
  - `cpu_stepping` - **string**: the CPU stepping.
  - `cpu_frequency` - **number/float**: the CPU frequency.
  - `cpu_cache_size` - **int**: the CPU cache size in bytes (filled on Linux and macOS, 0 on Windows).
  - `cpu_architecture` - **string**: the hardware name, Linux and macOS only (ex "x86_64", "arm64", "unknown", ...).
  - `kernel_name` - **string**: the kernel name (ex: "windows", "Linux", ...).
  - `kernel_release` - **string**:  the kernel release (ex: "10.0.20348", "4.15.0-1080-gcp", ...).
  - `kernel_version` - **string**:  the kernel version (Unix only, empty string on Windows).
  - `os` - **string**: the OS name description (ex: "GNU/Linux", "Windows Server 2022 Datacenter", ...).
  - `os_version` - **string**: the OS version (ex: "debian bookworm/sid", ...).
  - `memory_total_kb` - **int**: the total memory size for the host in KiB.
  - `memory_swap_total_kb` - **int**: the `swap` memory size in KiB (Unix only).
  - `ip_address` - **string**: the IP address for the host. When `metadata_ip_resolution_from_hostname` is true, this is
    the first IPv4 address the Agent hostname resolves to.
  - `ipv6_address` - **string**: the IPV6 address for the host. When `metadata_ip_resolution_from_hostname` is true,
    this is the first IPv6 address the Agent hostname resolves to.
  - `mac_address` - **string**: the MAC address for the host.
  - `interfaces` - **string**: a JSON-encoded list of the host network interfaces. Each interface has a `name` and,
    when available, `ipv4` (list of string), `ipv4-network`, `ipv6` (list of string), `ipv6-network` and `macaddress`.
  - `agent_version` - **string**: the version of the Agent that sent this payload.
  - `cloud_provider` - **string**: the name of the cloud provider detected by the Agent.
  - `cloud_provider_source` - **string**: the data source used to know that the Agent is running on `cloud_provider`.
    This is different for each cloud provider. For now ony AWS is supported.
    Values on AWS:
    - `IMDSv2`: The Agent successfully contacted IMDSv2 metadata endpoint.
    - `IMDSv1`: The Agent successfully contacted IMDSv1 metadata endpoint.
    - `DMI`: The Agent successfully used DMI information to fetch the instance ID (only works on Unix EC2 Nitro instances).
    - `UUID`: The hypervisor or product UUID has the EC2 prefix. The Agent knows it's running on EC2 but doesn't know
      which instance (see `hypervisor_guest_uuid` or `dmi_product_uuid`).
  - `cloud_provider_account_id` - **string**: The account/subscription ID from the cloud provider. Only collected when
    `inventories_collect_cloud_provider_account_id` is true (default).
  - `cloud_provider_host_id` - **string**: the unique ID the cloud provider uses to reference this instance.
    This is different for each cloud provider (for now, ony AWS is supported).
    - On AWS: the instance ID returned by querying the IMDSv2 endpoint. An empty string is returned if we can't reach
      IMDSv2 (even if IMDSv1 is available).
  - `ccrid` - **string**: the Canonical Cloud Resource Identifier for the host provided by the cloud provider. Supported
    on AWS, Azure, GCP and Oracle; empty string otherwise.
  - `hypervisor_guest_uuid` - **string**: the hypervisor guest UUID (Unix only, empty string on Windows or if we can't
    read the data). On `ec2` instances, this might start with "ec2". This was introduced in `7.41.0`/`6.41.0`.
  - `dmi_product_uuid` - **string**: the DMI product UUID (Unix only, empty string on Windows or if we can't read the
    data). On `ec2` instances, this might start with "ec2". This was introduced in `7.41.0`/`6.41.0`.
  - `dmi_board_asset_tag` - **string**: the DMI board tag (Unix only, empty string on Windows or if we can't read the
    data). On `ec2` Nitro instances, this contains the EC2 instance ID. This was introduced in `7.41.0`/`6.41.0`.
  - `dmi_board_vendor` - **string**: the DMI board vendor (Unix only, empty string on Windows or if we can't read the
    data). On `ec2` Nitro instances, this might equal "Amazon EC2". This was introduced in `7.41.0`/`6.41.0`.
  - `linux_package_signing_enabled` - **boolean**: is true if package signing is enabled on the host
    - It checks the presence of `no-debsig` in `/etc/dpkg/dpkg.cfg` for hosts relying on APT as package manager.
    - It checks the value of `gpgcheck` in the `[main]` repo file definition for distributions using YUM, DNF
      or ZYPPER as package manager.
  - `rpm_global_repo_gpg_check_enabled` - **boolean**: reflects the value of `repo_gpgcheck` in the `[main]` repo file
    of hosts relying on YUM, DNF or ZYPPER as package manager
  - `instance-type` - **string**: represents the type of instance for the host based on the cloud provider (instance type for AWS, machine type for GCP, VM size for Azure and instance shape for Oracle).

## Example Payload

Here an example of an inventory payload:

```json
{
    "host_metadata": {
        "cpu_architecture": "x86_64",
        "cpu_cache_size": 9437184,
        "cpu_cores": 6,
        "cpu_family": "6",
        "cpu_frequency": 2208.007,
        "cpu_logical_processors": 6,
        "cpu_model": "Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz",
        "cpu_model_id": "158",
        "cpu_stepping": "10",
        "cpu_vendor": "GenuineIntel",
        "kernel_name": "Linux",
        "kernel_release": "5.16.0-6-amd64",
        "kernel_version": "#1 SMP PREEMPT Debian 5.16.18-1 (2022-03-29)",
        "os": "GNU/Linux",
        "os_version": "debian bookworm/sid",
        "memory_swap_total_kb": 10237948,
        "memory_total_kb": 12227556,
        "ip_address": "192.168.1.10",
        "ipv6_address": "fe80::1ff:fe23:4567:890a",
        "mac_address": "01:23:45:67:89:AB",
        "interfaces": "[{\"ipv4\":[\"192.168.1.10\"],\"ipv4-network\":\"192.168.1.0/24\",\"ipv6\":[\"fe80::1ff:fe23:4567:890a\"],\"ipv6-network\":\"fe80::/64\",\"macaddress\":\"01:23:45:67:89:ab\",\"name\":\"eth0\"}]",
        "agent_version": "7.37.0-devel+git.198.68a5b69",
        "cloud_provider": "AWS",
        "cloud_provider_source": "IMDSv2",
        "cloud_provider_account_id": "123456789012",
        "cloud_provider_host_id": "i-abcedf",
        "ccrid": "arn:aws:ec2:us-east-1:123456789012:instance/i-abcedf",
        "hypervisor_guest_uuid": "ec200000-0000-0000-0000-000000000001",
        "dmi_product_uuid": "ec200000-0000-0000-0000-000000000001",
        "dmi_board_asset_tag": "i-abcedf",
        "dmi_board_vendor": "Amazon EC2",
        "linux_package_signing_enabled": true,
        "rpm_global_repo_gpg_check_enabled": false,
        "instance-type": "m5.large"
    },
    "hostname": "my-host",
    "uuid": "ec200000-0000-0000-0000-000000000001",
    "timestamp": 1631281754507358895
}
```
