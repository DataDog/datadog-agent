# Configuration Options

This page details the configuration options supported in Agent version 6 or later
compared to version 5. If you don't see a configuration option listed here, this
might mean:

 * The option is supported as it is, in this case you can find it in the [example file][datadog-yaml]
 * The option refers to a feature that's currently under development
 * The option refers to a feature that's scheduled but will come later

## New options in version 6

This is the list of configuration options that are either new, renamed or changed
in any way.

| Old Name | New Name | Notes |
| --- | --- | --- |
| `proxy_host`  | `proxy`  | Proxy settings are now expressed as a list of URIs like `http://user:password@proxyurl:port` |
| `collect_instance_metadata` | `enable_metadata_collection` | This now enabled the new metadata collection mechanism |
| `collector_log_file` | `log_file` ||
| `syslog_host`  | `syslog_uri`  | The Syslog configuration is now expressed as an URI |


## Removed options

This is the list of configuration options that were removed in the new Agent
beacause either superseded by new options or related to features that works
differently from Agent version 5.

| Name | Notes |
| --- | --- |
| `proxy_port` | superseded by `proxy` |
| `proxy_user` | superseded by `proxy` |
| `proxy_password` | superseded by `proxy` |
| `proxy_forbid_method_switch` | obsolete |
| `use_mount` | deprecated in v5 |
| `use_curl_http_client` | obsolete |
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
| `gce_updated_hostname` | |



[datadog-yaml]: https://raw.githubusercontent.com/DataDog/datadog-agent/master/pkg/collector/dist/datadog.yaml