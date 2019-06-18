# Configuration Options

This page details the configuration options supported in Agent version 6 or later
compared to version 5. If you don't see a configuration option listed here, this
might mean:

 * The option is supported as it is, in this case you can find it in the [example file][datadog-yaml]
 * The option refers to a feature that's currently under development
 * The option refers to a feature that's scheduled but will come later

## Environment variables

All the options supported by the Agent in the main configuration file (`datadog.yaml`) can also be set through environment variables, using the following rules:

1. Option names should be put in uppercase with the `DD_` prefix. Example: `hostname` -> `DD_HOSTNAME`

2. The nesting of config options should be indicated with an underscore separator. Example:
   ```yaml
   cluster_agent:
     cmd_port: <some_value>
   ```
   -> `DD_CLUSTER_AGENT_CMD_PORT=<some_value>`

   Exception: at the moment, only some of the options nested under `apm_config` and `process_config` can be set through environment variables.

3. List of values should be separated by spaces. Example:
   ```yaml
   ac_include:
     - "image:cp-kafka"
     - "image:k8szk"
   ```
   -> `DD_AC_INCLUDE="image:cp-kafka image:k8szk"`

4. Options that expect a map structure with _arbitrary_, _user-defined_ keys should be json-formatted. Example:
   ```yaml
   docker_env_as_tags:
     ENVVAR_NAME: tag_name
   ```
   -> `DD_DOCKER_ENV_AS_TAGS='{ "ENVVAR_NAME": "tag_name" }'`

   Note: this rule does not apply to options that expect a map structure with _predefined_ keys. For these options, refer to rule `2.` above.

Note: for a given config option, specifying a nested option with an environment variable overrides _all_ the nested options that are specified under that config option in the configuration file. There is one exception to this: the `proxy` config option, please refer to [the dedicated Agent proxy documentation](https://docs.datadoghq.com/agent/proxy/#agent-v6) for more on the proxy configuration.

## Orchestration + Agent Management

Orchestration has now been deferred to OS facilities wherever possible. To this purpose
we now rely on upstart/systemd on linux environments and windows services on Windows.
Enabling the APM and Process agents bundled with the agent can now be achieved via
configuration flags defined in the main configuration file: `datadog.yaml`.

### Process Agent
To enable the process agent add the following to `datadog.yaml`:
```
...
process_config:
  enabled: true
...
```

### Trace Agent
To enable the trace agent add the following to `datadog.yaml`:
```
...
apm_config:
  enabled: true
...
```

The OS-level services will be enabled by default for all agents. The agents will process
the configuration and decide whether to stay up or gracefully shut down. You may decide
to disable the OS-level service units, but that will require your manual intervention if
you ever wish to re-enable any of the agents.


## Changed options in version 6

This is the list of configuration options that are either new, renamed or changed
in any way.

| Old Name | New Name | Notes |
| --- | --- | --- |
| `proxy_host`  | `proxy`  | Proxy settings are now expressed as a list of URIs like `http://user:password@proxyurl:port`, one per transport type (see the `proxy` section of [datadog.yaml][datadog-yaml] for more details). |
| `collect_instance_metadata` | `enable_metadata_collection` | This now enabled the new metadata collection mechanism |
| `collector_log_file` | `log_file` ||
| `syslog_host`  | `syslog_uri`  | The Syslog configuration is now expressed as an URI |
|| `syslog_pem`  | Syslog configuration client certificate for TLS client validation |
|| `syslog_key`  | Syslog configuration client private key for TLS client validation |
| `DD_TAGS` | `DD_TAGS` | The format is space-separated, i.e. `simple-tag-0 tag-key-1:tag-value-1` |


## Integrations instance configuration

In addition to integration-specific options, the agent supports the following
advanced options in the `instance` section:

* `min_collection_interval`: set a different run interval in seconds, for checks
that should run less frequently than the default 15 seconds interval
* `empty_default_hostname`: submit metrics, events and service checks with no
hostname when set to `true`
* `tags`: send custom tags in addition to the tags sent by the check.

## Removed options

This is the list of configuration options that were removed in the new Agent
because they're either:
* superseded by new options, or
* related to features that work differently from Agent version 5

| Name | Notes |
| --- | --- |
| `proxy_port` | superseded by `proxy` |
| `proxy_user` | superseded by `proxy` |
| `proxy_password` | superseded by `proxy` |
| `proxy_forbid_method_switch` | obsolete |
| `use_mount` | deprecated in v5 |
| `use_curl_http_client` | obsolete |
| `exclude_process_args` | deprecated feature |
| `check_timings` | superseded by internal stats |
| `dogstatsd_target` | |
| `dogstreams` | |
| `custom_emitters` | |
| `forwarder_log_file` | superseded by `log_file` |
| `dogstatsd_log_file` | superseded by `log_file` |
| `jmxfetch_log_file` | superseded by `log_file` |
| `syslog_port` | superseded by `syslog_uri` |
| `check_freq` | |
| `collect_orchestrator_tags` | feature now implemented in metadata collectors |
| `utf8_decoding` | |
| `developer_mode` | |
| `use_forwarder` | |
| `autorestart` | |
| `dogstream_log` | |
| `use_curl_http_client` | |
| `gce_updated_hostname` | v6 behaves like v5 with `gce_updated_hostname` set to true. May affect reported hostname, see [doc][gce-hostname] |
| `collect_security_groups` | v6 doesn't collect security group host tags, feature is still available with the aws integration  |

[datadog-yaml]: https://raw.githubusercontent.com/DataDog/datadog-agent/master/pkg/config/config_template.yaml
[gce-hostname]: changes.md#gce-hostname
