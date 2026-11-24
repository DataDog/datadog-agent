# Host Metadata Payload (aka "v5" payload)

This package populates the `host` metadata payload, also known as the "v5" payload because its format mimics the
payload sent by Agent v5 (it was ported from Python to Go when Agent v6 started). The backend uses it to learn about
the host, including its aliases and host tags, and to populate products such as the Infrastructure List.

The payload is enabled by default and can only be turned off with `enable_metadata_collection: false`, which stops
every metadata payload. It is sent to the v1 intake (`/intake/`) at startup, then at an increasing interval
until it reaches the collection interval: about 5 minutes, then about 15 minutes, then every 30 minutes. Each interval
is randomized by up to 20% and never exceeds the collection interval, so Agents that start together don't send their
payloads in lockstep.

You can change both intervals with `metadata_providers`. Values are in seconds and must be between 60 and 14400
(4 hours). An out-of-range `interval` is ignored along with `early_interval`, and the Agent logs an error.
`early_interval` is optional: when it is omitted, zero or negative, the default early interval is kept without any
error. When it is set, it must be in the same range and not greater than `interval`, otherwise it is ignored (but
`interval` is still applied) and the Agent logs an error.

```yaml
metadata_providers:
  - name: host
    interval: 1800
    early_interval: 300
```

The payload contains the API key, so the forwarder never stores it on disk. It cannot be split, so the forwarder drops
it if it is too large.

# Content

The payload is built from scratch on every collection. Some values are collected only once and cached (system stats,
UUID, network ID). The latest payload is also cached for other parts of the Agent, such as the status page.

The `resources` and `gohai` fields depend on gohai. They are only included when the Agent is built for `linux`,
`windows`, `darwin` or `aix`. A fallback without these fields exists for `freebsd`, `netbsd`, `openbsd`, `solaris` and
`dragonfly` (`impl/payload_no_gohai.go`), but it currently doesn't compile, so the Agent can't send this payload on
those platforms.

# Format

The payload is a JSON dict with the following fields:

- `apiKey` - **string**: the main API key of the Agent (`api_key`, trimmed of whitespace). API keys from
  `additional_endpoints` are not included.
- `agentVersion` - **string**: the Agent version (ex: `7.75.0`).
- `uuid` - **string**: the host ID provided by the OS. On Unix this is the `HostID` from gopsutil; on Windows it is
  the `MachineGuid` value in the `HKLM\SOFTWARE\Microsoft\Cryptography` registry key.
- `internalHostname` - **string**: the Agent hostname, as shown on the status page.
- `os` - **string**: `"win32"` on Windows, Go's `runtime.GOOS` on other OSes (ex: `"linux"`, `"darwin"`).
- `agent-flavor` - **string**: the flavor of the running Agent (ex: `agent`, `iot_agent`, `cluster_agent`,
  ...). See `pkg/util/flavor`.
- `python` - **string**: the full Python version used by the Agent (ex:
  `"3.13.7 (main, Oct 10 2025, 13:16:08) [GCC 11.4.0]"`). This is `"unused"` while the Python runtime hasn't been
  loaded, and `"n/a"` if the Agent was built without Python support.
- `systemStats` - **dict**:
  - `cpuCores` - **int**: the number of CPU cores (gopsutil on Unix, gohai on Windows).
  - `machine` - **string**: the architecture, equal to Go's `runtime.GOARCH` (ex: `amd64`, `arm64`).
  - `platform` - **string**: equal to Go's `runtime.GOOS`, so `"windows"` on Windows (unlike the top-level `os`).
  - `pythonV` - **string**: the short Python version (ex: `3.13.7`). Same `"unused"`/`"n/a"` values as `python`.
  - `processor` - **string**: the CPU model (ex: `Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz`).
  - `macV`, `nixV`, `fbsdV`, `winV` - **list**: legacy OS version lists from Agent v5 (see [OS version lists](#os-version-lists)).
- `meta` - **dict**:
  - `socket-hostname` - **string**: the OS hostname (Go's `os.Hostname()`).
  - `timezones` - **list of string**: a single-item list with the local timezone abbreviation (ex: `["UTC"]`).
  - `socket-fqdn` - **string**: the FQDN of the host.
  - `ec2-hostname` - **string**: the EC2 hostname, or an empty string when not on EC2.
  - `hostname` - **string**: the Agent hostname, as shown on the status page.
  - `host_aliases` - **list of string**: the host aliases collected from cloud providers and the configuration
    (`host_aliases`). Can be `null` when there are none.
  - `instance-id` - **string**: the EC2 instance ID, or an empty string when not on EC2.
  - `agent-hostname` - **string** (omitted if empty): set to the configured hostname when
    `hostname_force_config_as_canonical` is `true` and the hostname comes from the `hostname` configuration.
  - `cluster-name` - **string** (omitted if empty): the Kubernetes cluster name. If the name isn't RFC1123-compliant,
    this is `"<compliant name> (original name: <name>)"`.
  - `legacy-resolution-hostname` - **string** (omitted if empty): the hostname the Agent would have resolved without
    IMDSv2/DMI. This is only set when it differs from `hostname`, during the IMDSv2 hostname transition
    (`ec2_imdsv2_transition_payload_enabled` set, `ec2_prefer_imdsv2` not set).
  - `hostname-resolution-version` - **int**: the version of the hostname resolution logic. Currently always `1`.
  - `ccrid` - **string** (omitted if empty): the Canonical Cloud Resource Identifier of the host from the detected
    cloud provider. Only collected when `collect_ccrid` is `true` (default).
- `host-tags` - **dict**:
  - `system` - **list of string**: the sorted and deduplicated host tags. They come from the configuration (`tags`,
    `extra_tags`, `env`), Kubernetes (cluster name, distribution, node labels and annotations as tags), the Cluster
    Agent, EC2 (`collect_ec2_tags`, `collect_ec2_instance_info`), Docker, and GPUs (`collect_gpu_tags`). The list
    also includes `private_action_runner_enabled:true` when the private action runner is enabled, and `infra_mode:<mode>`
    when `infrastructure_mode` is not `full` (EUDM mode also adds hardware tags on macOS and Windows). Values are split
    according to `tag_value_split_separator`.
  - `google cloud platform` - **list of string** (omitted if empty): the GCE host tags (`collect_gce_tags`).
- `container-meta` - **dict of string to string** (omitted if empty): metadata about the detected container runtimes.
  Possible keys: `docker_version`, `docker_swarm` (`"active"`/`"inactive"`), `kubelet_version`, `cri_name`,
  `cri_version`. Collection has a 1 second timeout.
- `network` - **dict** (`null` if no network ID can be found):
  - `network-id` - **string**: the network ID from the configuration (`network.id`) or detected on GCE/EC2.
  - `public-ipv4` - **string** (omitted if empty): the public IPv4 of the host on supported cloud providers.
- `logs` - **dict**:
  - `transport` - **string**: the transport used by the logs Agent: `"HTTP"`, `"TCP"`, or `""` if the logs Agent is
    disabled.
  - `auto_multi_line_detection_enabled` - **boolean**: the value of `logs_config.auto_multi_line_detection`.
- `install-method` - **dict**: how the Agent was installed, read from the `install_info` file written by the
  installation tool.
  - `tool` - **string**: the tool used to install the Agent (ex: `chef`, `ansible`, `install_script`, ...). `null`
    if `install_info` can't be read.
  - `tool_version` - **string**: the tool version. `"undefined"` if `install_info` can't be read.
  - `installer_version` - **string**: the version of the Datadog installation package (ex: the Chef cookbook or
    Ansible role). `null` if `install_info` can't be read.
- `proxy-info` - **dict**:
  - `no-proxy-nonexact-match` - **boolean**: the value of `no_proxy_nonexact_match`, or `false` if it isn't explicitly
    configured.
  - `proxy-behavior-changed` - **boolean**: `true` if enabling `no_proxy_nonexact_match` would change which requests
    go through the proxy.
  - `no-proxy-nonexact-match-explicitly-set` - **boolean**: `true` if `no_proxy_nonexact_match` is explicitly set in
    the configuration.
- `otlp` - **dict**:
  - `enabled` - **boolean**: `true` if OTLP ingestion is enabled in the Agent.
- `fips_mode` - **boolean**: `true` if the Agent runs in FIPS mode (FIPS-compliant build).
- `fips_proxy_enabled` - **boolean**: `true` if `fips.enabled` is set (FIPS proxy) and the Agent is not running in
  FIPS mode.
- `resources` - **dict** (omitted if gohai fails to collect process information; not sent on BSD and Solaris):
  - `processes` - **dict**:
    - `snaps` - **list**: a single process snapshot captured by gohai: `[<timestamp>, [<process group>, ...]]`, with
      the 20 process groups (processes grouped by name) using the most RSS. Each process group is
      `[<users>, <pct cpu>, <pct mem>, <vms>, <rss>, <name>, <process count>]`: `users` is a comma-separated list of
      usernames, `pct cpu` is an integer, `pct mem` a float, and `vms`/`rss` are in bytes.
  - `meta` - **dict**:
    - `host` - **string**: the Agent hostname.
- `gohai` - **string** (not sent on BSD and Solaris): a JSON object, serialized as a string, with gohai information.
  The object has the keys `cpu`, `filesystem`, `memory`, `network` and `platform`. `network` is `null` when the Agent is
  containerized without host networking, unless `kubernetes_kubelet_host` provides a node IP. This field is an empty
  string when `enable_gohai` is `false`. The standalone gohai payload, including processes, is available at
  `agent diagnose show-metadata gohai`.

## OS version lists

`macV`, `nixV`, `fbsdV` and `winV` are legacy fields from Agent v5, where they contained the output of Python's
`platform.mac_ver()`, `platform.dist()` and `platform.win32_ver()`. Only the list for the current OS is filled; the
other lists keep their default values. Their type depends on the OS the Agent was built for:

- **Linux and other Unix**: `nixV` is `[<platform>, <platform version>, ""]` (ex: `["ubuntu", "24.04", ""]`).
  `macV`, `fbsdV` and `winV` are `["", "", ""]`.
- **macOS**: `macV` is `[<platform version>, ["", "", ""], <GOARCH>]` (ex: `["15.1", ["", "", ""], "arm64"]`).
  `nixV`, `fbsdV` and `winV` are `["", "", ""]`.
- **Windows**: each list has 2 items. `winV` is `[<OS name>, <platform version>]`; `macV`, `nixV` and `fbsdV` are
  `["", ""]`.
- `fbsdV` is never filled.

The backend expects these exact shapes, so don't change them without coordinating with the backend team.

## Example Payload

Here is an example of a host payload from a Linux host. It includes every documented field, including conditional
fields that a single host would not send together (ex: `ec2-hostname` and `google cloud platform`). The process snapshot
is shortened:

```json
{
    "apiKey": "***************************aaaaa",
    "agentVersion": "7.75.0",
    "uuid": "00000000-0000-0000-0000-000000000001",
    "internalHostname": "i-0123456789abcdef0",
    "os": "linux",
    "agent-flavor": "agent",
    "python": "3.13.7 (main, Oct 10 2025, 13:16:08) [GCC 11.4.0]",
    "systemStats": {
        "cpuCores": 4,
        "machine": "amd64",
        "platform": "linux",
        "pythonV": "3.13.7",
        "processor": "Intel(R) Xeon(R) Platinum 8259CL CPU @ 2.50GHz",
        "macV": ["", "", ""],
        "nixV": ["ubuntu", "24.04", ""],
        "fbsdV": ["", "", ""],
        "winV": ["", "", ""]
    },
    "meta": {
        "socket-hostname": "ip-172-31-10-20",
        "timezones": ["UTC"],
        "socket-fqdn": "ip-172-31-10-20.ec2.internal",
        "ec2-hostname": "ip-172-31-10-20.ec2.internal",
        "hostname": "i-0123456789abcdef0",
        "host_aliases": ["my-host-alias"],
        "instance-id": "i-0123456789abcdef0",
        "agent-hostname": "my-configured-hostname",
        "cluster-name": "prod-cluster",
        "legacy-resolution-hostname": "ip-172-31-10-20",
        "hostname-resolution-version": 1,
        "ccrid": "arn:aws:ec2:us-east-1:123456789012:instance/i-0123456789abcdef0"
    },
    "host-tags": {
        "system": [
            "cluster_name:prod-cluster",
            "env:prod",
            "kube_cluster_name:prod-cluster",
            "team:infra"
        ],
        "google cloud platform": [
            "zone:us-central1-a"
        ]
    },
    "container-meta": {
        "cri_name": "containerd",
        "cri_version": "1.7.27",
        "docker_swarm": "inactive",
        "docker_version": "28.5.1",
        "kubelet_version": "v1.33.5"
    },
    "network": {
        "network-id": "vpc-0a1b2c3d4e5f67890",
        "public-ipv4": "203.0.113.10"
    },
    "logs": {
        "transport": "HTTP",
        "auto_multi_line_detection_enabled": false
    },
    "install-method": {
        "tool": "install_script",
        "tool_version": "install_script",
        "installer_version": "install_script-1.40.0"
    },
    "proxy-info": {
        "no-proxy-nonexact-match": false,
        "proxy-behavior-changed": false,
        "no-proxy-nonexact-match-explicitly-set": false
    },
    "otlp": {
        "enabled": false
    },
    "fips_mode": false,
    "fips_proxy_enabled": false,
    "resources": {
        "processes": {
            "snaps": [
                [
                    1763650000,
                    [
                        ["dd-agent", 2, 0.8, 1730998272, 132431872, "agent", 1],
                        ["root", 0, 0.04, 10354688, 6422528, "sshd", 3]
                    ]
                ]
            ]
        },
        "meta": {
            "host": "i-0123456789abcdef0"
        }
    },
    "gohai": "{\"cpu\":{\"cache_size\":\"36608 KB\",\"cpu_cores\":\"2\",\"cpu_logical_processors\":\"4\",\"family\":\"6\",\"mhz\":\"2500.000\",\"model\":\"85\",\"model_name\":\"Intel(R) Xeon(R) Platinum 8259CL CPU @ 2.50GHz\",\"stepping\":\"7\",\"vendor_id\":\"GenuineIntel\"},\"filesystem\":[{\"kb_size\":\"7941576\",\"mounted_on\":\"/\",\"name\":\"/dev/root\"}],\"memory\":{\"swap_total\":\"0kB\",\"total\":\"16107032kB\"},\"network\":{\"interfaces\":[{\"name\":\"ens5\",\"ipv4\":[\"172.31.10.20\"],\"ipv4-network\":\"172.31.0.0/20\",\"ipv6\":[\"fe80::1ff:fe23:4567:890a\"],\"ipv6-network\":\"fe80::/64\",\"macaddress\":\"0a:1b:2c:3d:4e:5f\"}],\"ipaddress\":\"172.31.10.20\",\"ipaddressv6\":\"fe80::1ff:fe23:4567:890a\",\"macaddress\":\"0a:1b:2c:3d:4e:5f\"},\"platform\":{\"GOOARCH\":\"amd64\",\"GOOS\":\"linux\",\"goV\":\"1.25.3\",\"hostname\":\"ip-172-31-10-20\",\"kernel_name\":\"Linux\",\"kernel_release\":\"6.8.0-1015-aws\",\"kernel_version\":\"#16-Ubuntu SMP Mon Aug 19 19:38:17 UTC 2024\",\"machine\":\"x86_64\",\"os\":\"GNU/Linux\",\"processor\":\"x86_64\",\"hardware_platform\":\"x86_64\",\"family\":\"\"}}"
}
```

## Inspecting the payload

- `agent diagnose show-metadata v5` prints the current payload (scrubbed) from a running Agent.
- `agent diagnose show-metadata gohai` prints the full gohai payload, including processes.
- Flares include the payload in `metadata/host.json`.
