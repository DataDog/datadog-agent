// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

// DefaultPolicy holds the default runtime security agent rules
var DefaultPolicy = `---
version: 1.0.1
rules:
  - id: credential_accessed
    description: Sensitive credential files were accessed using a non-standard tool
    expression: >-
      (open.filename == "/etc/shadow" || open.filename == "/etc/gshadow") &&
      process.name not in ["vipw", "vigr", "accounts-daemon"]
    tags:
      technique: T1003
  - id: memory_dump
    description: Potential memory dump
    expression: >-
      open.filename =~ "/proc/*" && open.basename in ["maps", "mem"]
    tags:
      technique: T1003
  - id: logs_altered
    description: Log data was deleted
    expression: >-
      (open.filename =~ "/var/log/*" && open.flags & O_TRUNC > 0) &&
      process.name not in ["agent", "security-agent", "process-agent", "system-probe", "kubelet", "containerd"]
    tags:
      technique: T1070
  - id: logs_removed
    description: Log files were removed
    expression: >-
      unlink.filename =~ "/var/log/*" &&
      process.name not in ["agent", "security-agent", "process-agent", "system-probe", "kubelet", "containerd"]
    tags:
      technique: T1070
  - id: permissions_changed
    description: Permissions were changed on sensitive files
    expression: >-
      (chmod.filename =~ "/etc/*" ||
      chmod.filename =~ "/sbin/*" || chmod.filename =~ "/usr/sbin/*" ||
      chmod.filename =~ "/usr/local/sbin/*" || chmod.filename =~ "/usr/local/bin/*" ||
      chmod.filename =~ "/var/log/*" || chmod.filename =~ "/usr/lib/*") &&
      process.name not in ["containerd", "kubelet"]
    tags:
      technique: T1222
  - id: kernel_module
    description: A new kernel module was added
    expression: >-
      (open.filename =~ "/lib/modules/*" || open.filename =~ "/usr/lib/modules/*") && open.flags & O_CREAT > 0
    tags:
      technique: T1215
  - id: nsswitch_conf_mod
    description: Exploits that modify nsswitch.conf to interfere with authentication
    expression: >-
      open.filename == "/etc/nsswitch.conf" && open.flags & (O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1556
  - id: pam_modification
    description: PAM modification
    expression: >-
      open.filename =~ "/etc/pam.d/*" && open.flags & (O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1556
  - id: cron_at_job_injection
    description: Unauthorized scheduling client
    expression: >-
      open.filename =~ "/var/spool/cron/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0 &&
      process.name not in ["at", "crontab"]
    tags:
      technique: T1053
  - id: kernel_modification
    description: Unauthorized kernel modification
    expression: >-
      open.filename =~ "/boot/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1014
  - id: systemd_modification
    description: Unauthorized modification of a service
    expression: >-
      open.filename =~ "/usr/lib/systemd/system/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1014
  - id: authentication_logs_accessed
    description: unauthorized file accessing access logs
    expression: >-
      open.filename in ["/run/utmp", "/var/run/utmp", "/var/log/wtmp"] &&
      process.name not in ["login", "sshd", "last", "who", "w", "vminfo"]
    tags:
      technique: T1070
  - id: root_ssh_key
    description: attempts to create or modify root's SSH key
    expression: >-
      open.filename == "/root/.ssh/authorized_keys" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1556
  - id: ssl_certificate_tampering
    description: Tampering with SSL certificates for machine-in-the-middle attacks against OpenSSL
    expression: >-
      open.filename =~ "/etc/ssl/certs/*" && open.flags & (O_CREAT | O_RDWR | O_WRONLY) > 0
    tags:
      technique: T1338
  - id: pci_11_5
    description: Modification of critical system files
    expression: >-
      (open.filename =~ "/bin/*" ||
      open.filename =~ "/sbin/*" ||
      open.filename =~ "/usr/bin/*" ||
      open.filename =~ "/usr/sbin/*" ||
      open.filename =~ "/opt/*") && 
      open.flags & (O_RDWR | O_WRONLY) > 0 &&
      process.name not in ["agent", "security-agent", "system-probe", "process-agent"]
`
