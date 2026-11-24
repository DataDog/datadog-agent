# Inventory Checks Payload

This package populates some of the checks-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using the `inventories_enabled` or `enable_metadata_collection`
config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 1 minute (see `inventories_min_interval`).

# Content

The `Set` method from the component allow the rest of the codebase to add any Metadata to a check running in the collector.
Metadata can include the check version, the version of the monitored software, ... It depends on each check.

For every running check, no matter if it registered extra metadata or not, we send: name, ID, configuration provider
and configuration source. Sending the checks configuration (`init_config` and `instance_config`) and `logs_metadata` can
be disabled using `inventories_checks_configuration_enabled`. `files_metadata` is not affected by this setting.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `check_metadata` - **dict of string to list**: dictionary with check names as keys; values are a list of the metadata for each
  instance of that check.
  Each instance is composed of:
    - `config.hash` - **string**: the instance ID for this instance (as shown in the status page).
    - `config.provider` - **string**: where the configuration came from for this instance (disk, docker labels, ...).
    - `config.source` - **string**: the configuration source without its provider prefix (usually the file path), or
      `unknown` if the source has no prefix.
    - `init_config` - **string**: the scrubbed `init_config` part of the configuration for this check instance. Only sent
      when `inventories_checks_configuration_enabled` is true.
    - `instance_config` - **string**: the scrubbed YAML configuration for this check instance. Only sent when
      `inventories_checks_configuration_enabled` is true.
    - Any other metadata registered by the instance (instance version, version of the software monitored, ...).
- `logs_metadata` - **dict of string to list**: dictionary with the log source names as keys; values are a list of the metadata
  for each instance of that log source. Empty when `inventories_checks_configuration_enabled` is false.
  Each instance is composed of:
    - `config` - **string**: the canonical JSON of the log source configuration.
    - `state` - **dict of string**: the current state of the log source.
      - `status` - **string**: one of `pending`, `error` or `success`.
      - `error` - **string**: the error description if any.
    - `integration_name` - **string**: the name of the integration, can be empty.
    - `service` - **string**: the service name of the log source.
    - `source` - **string**: the log source name.
    - `tags` - **list of string**: a list of tags attached to the log source.
    - `integration_source` - **string**: the Autodiscovery configuration source this log source comes from (ex:
      `file:/etc/datadog-agent/conf.d/redisdb.d/conf.yaml`). Empty if the log source doesn't come from an integration.
    - `integration_source_index` - **int**: the index of this log configuration in the `logs` section of that source.
- `files_metadata` - **dict of string to object**: the list of files path to content of integrations. The key is the full
  path of the file.
  Each object is composed of:
    - `raw_config` - **string**: the scrubbed configuration file.
    - `hash` - **string**: sha256 of the scrubbed file.
    - `secrets` - **array of objects**: the secret declarations in the file (see
      [Secret declarations in configuration files](#secret-declarations-in-configuration-files)). Omitted when
      `metadata_skip_secrets` is true.
      - `kind` - **string**: `enc` or `raw`.
      - `path` - **string**: the dotted path of the setting, relative to the file.
      - `handle` - **string** (omitted for `raw`): the secret handle used in `ENC[]`.
- `enc_info` - **object**: the secrets declared across all checks. Omitted when `metadata_skip_secrets` is true.
    - `paths` - **list of string**: the paths of settings that contain secrets using the ENC directive.
    - `handles` - **list of string**: the handles that correspond to these paths.
    - `scrubbed` - **list of string**: the path of each raw credential replaced by the scrubber.
    - `hash_secret_id` - **dict of string to string**: the sha256 of the resolved value of each secret. The hashes are
      scrubbed from flares and from the `show-metadata` output.

### JMX checks

In Agents built with JMX support, JMXFetch check instances are also listed in `check_metadata`, with a different shape:

- `config.hash` is `<check name>-<host>-<port>`, followed by `:<name>` if the instance sets `name`.
- The instance YAML is under the key `instance` (not `instance_config`).
- `jmxfetch.version` and `java.version` are added when known.
- `init_config` and `instance` are sent regardless of `inventories_checks_configuration_enabled`.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```json
{
    "check_metadata": {
        "cpu": [
            {
                "config.hash": "cpu",
                "config.provider": "file",
                "config.source": "/etc/datadog-agent/conf.d/cpu.d/conf.yaml.default",
                "init_config": "",
                "instance_config": "{}"
            }
        ],
        "disk": [
            {
                "config.hash": "disk:e5dffb8bef24336f",
                "config.provider": "file",
                "config.source": "/etc/datadog-agent/conf.d/disk.d/conf.yaml.default",
                "init_config": "",
                "instance_config": "use_mount: false"
            }
        ],
        "redisdb": [
            {
                "config.hash": "redisdb:6e5e79e5b724c83a",
                "config.provider": "container",
                "config.source": "docker://4bc64d2a3e7f9c1b8d0e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c",
                "init_config": "test: 21",
                "instance_config": "host: localhost\nport: 6379\ntags:\n- docker_image:redis\n- image_name:redis\n- short_image:redis",
                "version.major": "7",
                "version.minor": "0",
                "version.patch": "2",
                "version.raw": "7.0.2",
                "version.scheme": "semver"
            }
        ],
        "tomcat": [
            {
                "config.hash": "tomcat-localhost-9012",
                "config.provider": "file",
                "config.source": "/etc/datadog-agent/conf.d/tomcat.d/conf.yaml",
                "init_config": "is_jmx: true\ncollect_default_metrics: true\n",
                "instance": "host: localhost\nport: 9012\n",
                "jmxfetch.version": "0.49.6",
                "java.version": "17.0.12"
            }
        ]
    },
    "logs_metadata": {
        "redisdb": [
            {
                "config": "{\"path\":\"/var/log/redis_6379.log\",\"service\":\"awesome_cache\",\"source\":\"redis\",\"type\":\"file\",\"tags\":[\"env:prod\"]}",
                "integration_name": "redisdb",
                "integration_source": "container:docker://4bc64d2a3e7f9c1b8d0e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c",
                "integration_source_index": 0,
                "service": "awesome_cache",
                "source": "redis",
                "state": {
                    "error": "Error: cannot read file /var/log/redis_6379.log: stat /var/log/redis_6379.log: no such file or directory",
                    "status": "error"
                },
                "tags": ["env:prod"]
            }
        ],
        "nginx": [
            {
                "config": "{\"path\":\"/var/log/nginx/access.log\",\"service\":\"nginx\",\"source\":\"nginx\",\"type\":\"file\"}",
                "integration_name": "nginx",
                "integration_source": "file:/etc/datadog-agent/conf.d/nginx.d/conf.yaml",
                "integration_source_index": 0,
                "service": "nginx",
                "source": "nginx",
                "state": {
                    "error": "",
                    "status": "success"
                },
                "tags": []
            }
        ]
    },
    "files_metadata": {
        "/etc/datadog-agent/conf.d/hello.d/conf.yaml": {
            "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
            "raw_config": "init_config: {}\ninstances:\n- min_collection_interval: 15\n  password: ENC[db_password]\n  headers:\n    Authorization: '********'\n",
            "secrets": [
                {"path": "instances.0.password", "kind": "enc", "handle": "db_password"},
                {"path": "instances.0.headers.Authorization", "kind": "raw"}
            ]
        }
    },
    "enc_info": {
        "paths": ["instances.0.password"],
        "handles": ["db_password"],
        "scrubbed": ["instances.0.headers.Authorization"],
        "hash_secret_id": {"db_password": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}
    },
    "hostname": "my-host",
    "uuid": "00000000-0000-0000-0000-000000000001",
    "timestamp": 1631281754507358895
}
```

### Secret declarations in configuration files

This metadata is intended to support Fleet Automation for VM-based Agents and only
covers configuration files reported in `files_metadata`. Secrets supplied only
through other configuration sources, such as environment variables or Kubernetes
Autodiscovery annotations, are not included. It is not a complete inventory of
secrets in the effective configuration.

Each `files_metadata[filename].secrets` array describes the secret declarations in
that file. Entries have `kind` (`enc` or `raw`), `path` (a dotted path relative to the file,
including list indexes), and, for ENC declarations, `handle`. For example,
`{"kind":"enc","path":"instances.0.password","handle":"db_password"}`.
The filename and path together identify a location, including when two files
configure the same integration. Values and hashes are never included in these entries.
Raw references identify the field replaced by a scrubbing rule. When a rule
replaces a list or mapping, the reference identifies that whole field (for example,
`snmp_traps.community_strings`), rather than reconstructing its original leaves.
ENC references always retain their original indexed paths and handles, even inside
a replaced container. Repeated handles retain one entry per location. These entries describe file
contents; `agent_metadata.enc_info` continues reporting resolved handles and hashes.
`enc_info` aggregates these entries across all files: paths are relative to each file and are not
prefixed by the filename, so the same path can appear several times.
`metadata_skip_secrets` suppresses these entries as well as `enc_info`.

Secret references are collected independently of the existing integration configuration
loader and scrubbed file output. If a configuration accepted by the YAML v2 loader
fails strict YAML validation, its file metadata is still reported without secret references.
