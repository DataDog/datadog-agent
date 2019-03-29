# StackState Agent v2 releases

## 2.0.3 ( ??? )

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


## 2.0.2 (2019-03-28)

**Improvements**

- Disable resource snaps collection _[(STAC-2915)](https://stackstate.atlassian.net/browse/STAC-2915)_
- Support CentOS 6 _[(STAC-4139)](https://stackstate.atlassian.net/browse/STAC-4278)_
