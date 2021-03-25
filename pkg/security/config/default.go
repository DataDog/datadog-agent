// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// DefaultPolicy holds the default runtime security agent rules
var DefaultPolicy = `---
version: 1.0.1
rules:
  - id: credential_accessed
    description: Sensitive credential files were accessed using a non-standard tool
    expression: >-
      (open.file.path == "/etc/shadow" || open.file.path == "/etc/gshadow") &&
      process.file.name not in ["vipw", "vigr", "accounts-daemon"]
    tags:
      technique: T1003
  - id: memory_dump
    description: Potential memory dump
    expression: >-
      open.file.path =~ "/proc/*" && open.file.name in ["maps", "mem"]
    tags:
      technique: T1003
  - id: logs_altered
    description: Log data was deleted
    expression: >-
      (open.file.path =~ "/var/log/*" && open.flags & O_TRUNC > 0) &&
      process.file.name not in ["agent", "security-agent", "process-agent", "system-probe", "kubelet", "containerd"]
    tags:
      technique: T1070
  - id: logs_removed
    description: Log files were removed
    expression: >-
      unlink.file.path =~ "/var/log/*" &&
      process.file.name not in ["agent", "security-agent", "process-agent", "system-probe", "kubelet", "containerd"]
    tags:
      technique: T1070
  - id: permissions_changed
    description: Permissions were changed on sensitive files
    expression: >-
      (chmod.file.path =~ "/etc/*" ||
      chmod.file.path =~ "/sbin/*" || chmod.file.path =~ "/usr/sbin/*" ||
      chmod.file.path =~ "/usr/local/sbin/*" || chmod.file.path =~ "/usr/local/bin/*" ||
      chmod.file.path =~ "/var/log/*" || chmod.file.path =~ "/usr/lib/*") &&
      process.file.name not in ["containerd", "kubelet"]
    tags:
      technique: T1222
  - id: kernel_module
    description: A new kernel module was added
    expression: >-
      (open.file.path =~ "/lib/modules/*" || open.file.path =~ "/usr/lib/modules/*") && open.flags & O_CREAT > 0
    tags:
      technique: T1215
  - id: nsswitch_conf_mod
    description: Exploits that modify nsswitch.conf to interfere with authentication
    expression: >-
      open.file.path == "/etc/nsswitch.conf" && open.flags & (O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1556
  - id: pam_modification
    description: PAM modification
    expression: >-
      open.file.path =~ "/etc/pam.d/*" && open.flags & (O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1556
  - id: cron_at_job_injection
    description: Unauthorized scheduling client
    expression: >-
      open.file.path =~ "/var/spool/cron/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0 &&
      process.file.name not in ["at", "crontab"]
    tags:
      technique: T1053
  - id: kernel_modification
    description: Unauthorized kernel modification
    expression: >-
      open.file.path =~ "/boot/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1014
  - id: systemd_modification
    description: Unauthorized modification of a service
    expression: >-
      open.file.path =~ "/usr/lib/systemd/system/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1014
  - id: authentication_logs_accessed
    description: unauthorized file accessing access logs
    expression: >-
      open.file.path in ["/run/utmp", "/var/run/utmp", "/var/log/wtmp"] &&
      process.file.name not in ["login", "sshd", "last", "who", "w", "vminfo"]
    tags:
      technique: T1070
  - id: root_ssh_key
    description: attempts to create or modify root's SSH key
    expression: >-
      open.file.path == "/root/.ssh/authorized_keys" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1556
  - id: ssl_certificate_tampering
    description: Tampering with SSL certificates for machine-in-the-middle attacks against OpenSSL
    expression: >-
      open.file.path =~ "/etc/ssl/certs/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1338
  - id: pci_11_5
    description: Modification of critical system files
    expression: >-
      (open.file.path =~ "/bin/*" ||
      open.file.path =~ "/sbin/*" ||
      open.file.path =~ "/usr/bin/*" ||
      open.file.path =~ "/usr/sbin/*" ||
      open.file.path =~ "/opt/*") &&
      open.flags & (O_RDWR | O_WRONLY) > 0 &&
      process.file.name not in ["agent", "security-agent", "system-probe", "process-agent"]
`
