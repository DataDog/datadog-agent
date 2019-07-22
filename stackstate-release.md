# StackState Agent v2 releases

## 2.0.4 ( ??? )

**Features**

- Add new stackstate-agent-integrations _[(STAC-4964)](https://stackstate.atlassian.net/browse/STAC-4964)_
- Add python bindings and handling of topology _[(STAC-4869)](https://stackstate.atlassian.net/browse/STAC-4869)_
- Enable new trace agent and propagate starttime, pid and hostname tags _[(STAC-4878)](https://stackstate.atlassian.net/browse/STAC-4878)_

## 2.0.3 (2019-05-28)

**Features**

- Filter reported processes _[(STAC-3401)](https://stackstate.atlassian.net/browse/STAC-3401)_

  This feature changed and extended the agent configuration.
  
  Under the `process_config` section we removed `blacklist_patterns` and introduced the following:
  
  ```
  process_blacklist:
    # A list of regex patterns that will exclude a process arguments if matched.
    patterns:
      - ...
    # Inclusions rules for blacklisted processes which reports high usage.
    inclusions:
      amount_top_cpu_pct_usage: 3
      cpu_pct_usage_threshold: 20
      amount_top_io_read_usage: 3
      amount_top_io_write_usage: 3
      amount_top_mem_usage: 3
      mem_usage_threshold: 35
  ```
  
  Those configurations can be provided through environment variables as well:
    
  | Parameter | Default | Description |
  |-----------|---------|-------------|
  | `STS_PROCESS_BLACKLIST_PATTERNS` | [see github](https://github.com/StackVista/stackstate-process-agent/blob/master/config/config_nix.go) | A list of regex patterns that will exclude a process if matched |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_CPU` | 0 | Number of processes to report that have a high CPU usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_IO_READ` | 0 | Number of processes to report that have a high IO read usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_IO_WRITE` | 0 | Number of processes to report that have a high IO write usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_MEM` | 0 | Number of processes to report that have a high Memory usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_CPU_THRESHOLD` |  | Threshold that enables the reporting of high CPU usage processes |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_MEM_THRESHOLD` |  | Threshold that enables the reporting of high Memory usage processes |

- Report localhost connections within the same network namespace _[(STAC-2891)](https://stackstate.atlassian.net/browse/STAC-2891)_
  
  This feature adds support to identify localhost connections within docker containers within the same network namespace.
  
  The network namespace of the reported connection can be observed in StackState on the connection between the components.

- Upstream upgrade to 6.10.2 _[(STAC-3220)](https://stackstate.atlassian.net/browse/STAC-3220)_

## 2.0.2 (2019-03-28)

**Improvements**

- Disable resource snaps collection _[(STAC-2915)](https://stackstate.atlassian.net/browse/STAC-2915)_
- Support CentOS 6 _[(STAC-4139)](https://stackstate.atlassian.net/browse/STAC-4139)_
