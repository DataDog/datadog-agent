// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

// DefaultPolicy holds the default runtime security agent rules
var DefaultPolicy = `---
version: 1.0.0
rules:
  - id: credential_modified
    description: credential files modified using unknown tool
    expression: >-
      (open.filename == "/etc/shadow" || open.filename == "/etc/gshadow") &&
      process.name not in ["vipw", "vigr"]
    tags:
      mitre: T1003
  - id: memory_dump
    description: memory dump
    expression: >-
      open.filename =~ "/proc/*" && open.basename in ["maps", "mem"]
    tags:
      mitre: T1003
  - id: logs_altered
    description: log entries removed
    expression: >-
      (open.filename =~ "/var/log/*" && open.flags & O_TRUNC > 0)
    tags:
      mitre: T1070
  - id: logs_removed
    description: log entries removed
    expression: >-
      unlink.filename =~ "/var/log/*" &&
      unlink.filename != "/var/log/datadog/system-probe.log"
    tags:
      mitre: T1070
  - id: permissions_changed
    description: permissions change on sensible files
    expression: >-
      chmod.filename =~ "/etc/*" || chmod.filename =~ "/etc/*" ||
      chmod.filename =~ "/sbin/*" || chmod.filename =~ "/usr/sbin/*" ||
      chmod.filename =~ "/usr/local/sbin/*" || chmod.filename =~ "/usr/local/bin/*" ||
      chmod.filename =~ "/var/log/*" || chmod.filename =~ "/usr/lib/*"
    tags:
      mitre: T1099
  - id: hidden_file
    description: hidden file creation
    expression: >-
      open.basename =~ ".*" && open.flags & O_CREAT > 0 &&
      open.filename !~ "/run/containerd/io.containerd.runtime.v1.linux/k8s.io/*" &&
      open.basename !~ ".*.pid" &&
      process.name != "runc"
    tags:
      mitre: T1158
  - id: kernel_module
    description: new file in kernel module location
    expression: >-
      (open.filename =~ "/usr/lib/modules/*" || open.filename =~ "/lib/modules/*") && open.flags & O_CREAT > 0
    tags:
      mitre: T1215
`
