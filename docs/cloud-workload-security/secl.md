# SECL Documentation

### Event types

| SECL Event | Type | Definition | Agent Version |
| ---------- | ---- | ---------- | ------------- |
| `capset` | Process | A process changed its capacity set | 7.27 |
| `chmod` | File | A file’s permissions were changed | 7.27 |
| `chown` | File | A file’s owner was changed | 7.27 |
| `exec` | Process | A process was executed or forked | 7.27 |
| `link` | File | Create a new name/alias for a file | 7.27 |
| `mkdir` | File | A directory was created | 7.27 |
| `open` | File | A file was opened | 7.27 |
| `removexattr` | File | Remove extended attributes | 7.27 |
| `rename` | File | A file/directory was renamed | 7.27 |
| `rmdir` | File | A directory was removed | 7.27 |
| `selinux` | Kernel | An SELinux operation was run | 7.30 |
| `setgid` | Process | A process changed its effective gid | 7.27 |
| `setuid` | Process | A process changed its effective uid | 7.27 |
| `setxattr` | File | Set exteneded attributes | 7.27 |
| `unlink` | File | A file was deleted | 7.27 |
| `utimes` | File | Change file access/modification times | 7.27 |


### Common to all event types

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `*.container.id` |  |  |
| `*.container.tags` |  |  |
| `*.process.ancestors.cap_effective` |  |  |
| `*.process.ancestors.cap_permitted` |  |  |
| `*.process.ancestors.comm` |  |  |
| `*.process.ancestors.container.id` |  |  |
| `*.process.ancestors.cookie` |  |  |
| `*.process.ancestors.created_at` |  |  |
| `*.process.ancestors.egid` |  |  |
| `*.process.ancestors.egroup` |  |  |
| `*.process.ancestors.euid` |  |  |
| `*.process.ancestors.euser` |  |  |
| `*.process.ancestors.file.filesystem` |  |  |
| `*.process.ancestors.file.gid` |  |  |
| `*.process.ancestors.file.group` |  |  |
| `*.process.ancestors.file.in_upper_layer` |  |  |
| `*.process.ancestors.file.inode` |  |  |
| `*.process.ancestors.file.mode` |  |  |
| `*.process.ancestors.file.mount_id` |  |  |
| `*.process.ancestors.file.name` |  |  |
| `*.process.ancestors.file.path` |  |  |
| `*.process.ancestors.file.rights` |  |  |
| `*.process.ancestors.file.uid` |  | uid field definition |
| `*.process.ancestors.file.user` |  |  |
| `*.process.ancestors.fsgid` |  |  |
| `*.process.ancestors.fsgroup` |  |  |
| `*.process.ancestors.fsuid` |  |  |
| `*.process.ancestors.fsuser` |  |  |
| `*.process.ancestors.gid` |  |  |
| `*.process.ancestors.group` |  |  |
| `*.process.ancestors.pid` |  |  |
| `*.process.ancestors.ppid` |  |  |
| `*.process.ancestors.tid` |  |  |
| `*.process.ancestors.tty_name` |  |  |
| `*.process.ancestors.uid` |  |  |
| `*.process.ancestors.user` |  |  |
| `*.process.cap_effective` |  |  |
| `*.process.cap_permitted` |  |  |
| `*.process.comm` |  |  |
| `*.process.container.id` |  |  |
| `*.process.cookie` |  |  |
| `*.process.created_at` |  |  |
| `*.process.egid` |  |  |
| `*.process.egroup` |  |  |
| `*.process.euid` |  |  |
| `*.process.euser` |  |  |
| `*.process.file.filesystem` |  |  |
| `*.process.file.gid` |  |  |
| `*.process.file.group` |  |  |
| `*.process.file.in_upper_layer` |  |  |
| `*.process.file.inode` |  |  |
| `*.process.file.mode` |  |  |
| `*.process.file.mount_id` |  |  |
| `*.process.file.name` |  |  |
| `*.process.file.path` |  |  |
| `*.process.file.rights` |  |  |
| `*.process.file.uid` |  | uid field definition |
| `*.process.file.user` |  |  |
| `*.process.fsgid` |  |  |
| `*.process.fsgroup` |  |  |
| `*.process.fsuid` |  |  |
| `*.process.fsuser` |  |  |
| `*.process.gid` |  |  |
| `*.process.group` |  |  |
| `*.process.pid` |  |  |
| `*.process.ppid` |  |  |
| `*.process.tid` |  |  |
| `*.process.tty_name` |  |  |
| `*.process.uid` |  |  |
| `*.process.user` |  |  |

### Event `capset`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `capset.cap_effective` |  |  |
| `capset.cap_permitted` |  |  |

### Event `chmod`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `chmod.file.destination.mode` |  |  |
| `chmod.file.destination.rights` |  |  |
| `chmod.file.filesystem` |  |  |
| `chmod.file.gid` |  |  |
| `chmod.file.group` |  |  |
| `chmod.file.in_upper_layer` |  |  |
| `chmod.file.inode` |  |  |
| `chmod.file.mode` |  |  |
| `chmod.file.mount_id` |  |  |
| `chmod.file.name` |  |  |
| `chmod.file.path` |  |  |
| `chmod.file.rights` |  |  |
| `chmod.file.uid` |  | uid field definition |
| `chmod.file.user` |  |  |
| `chmod.retval` |  |  |

### Event `chown`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `chown.file.destination.gid` |  |  |
| `chown.file.destination.group` |  |  |
| `chown.file.destination.uid` |  |  |
| `chown.file.destination.user` |  |  |
| `chown.file.filesystem` |  |  |
| `chown.file.gid` |  |  |
| `chown.file.group` |  |  |
| `chown.file.in_upper_layer` |  |  |
| `chown.file.inode` |  |  |
| `chown.file.mode` |  |  |
| `chown.file.mount_id` |  |  |
| `chown.file.name` |  |  |
| `chown.file.path` |  |  |
| `chown.file.rights` |  |  |
| `chown.file.uid` |  | uid field definition |
| `chown.file.user` |  |  |
| `chown.retval` |  |  |

### Event `exec`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `exec.args` |  |  |
| `exec.args_flags` |  |  |
| `exec.args_options` |  |  |
| `exec.args_truncated` |  |  |
| `exec.argv` |  |  |
| `exec.cap_effective` |  |  |
| `exec.cap_permitted` |  |  |
| `exec.comm` |  |  |
| `exec.container.id` |  |  |
| `exec.cookie` |  |  |
| `exec.created_at` |  |  |
| `exec.egid` |  |  |
| `exec.egroup` |  |  |
| `exec.envs` |  |  |
| `exec.envs_truncated` |  |  |
| `exec.euid` |  |  |
| `exec.euser` |  |  |
| `exec.file.filesystem` |  |  |
| `exec.file.gid` |  |  |
| `exec.file.group` |  |  |
| `exec.file.in_upper_layer` |  |  |
| `exec.file.inode` |  |  |
| `exec.file.mode` |  |  |
| `exec.file.mount_id` |  |  |
| `exec.file.name` |  |  |
| `exec.file.path` |  |  |
| `exec.file.rights` |  |  |
| `exec.file.uid` |  | uid field definition |
| `exec.file.user` |  |  |
| `exec.fsgid` |  |  |
| `exec.fsgroup` |  |  |
| `exec.fsuid` |  |  |
| `exec.fsuser` |  |  |
| `exec.gid` |  |  |
| `exec.group` |  |  |
| `exec.pid` |  |  |
| `exec.ppid` |  |  |
| `exec.tid` |  |  |
| `exec.tty_name` |  |  |
| `exec.uid` |  |  |
| `exec.user` |  |  |

### Event `link`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `link.file.destination.filesystem` |  |  |
| `link.file.destination.gid` |  |  |
| `link.file.destination.group` |  |  |
| `link.file.destination.in_upper_layer` |  |  |
| `link.file.destination.inode` |  |  |
| `link.file.destination.mode` |  |  |
| `link.file.destination.mount_id` |  |  |
| `link.file.destination.name` |  |  |
| `link.file.destination.path` |  |  |
| `link.file.destination.rights` |  |  |
| `link.file.destination.uid` |  | uid field definition |
| `link.file.destination.user` |  |  |
| `link.file.filesystem` |  |  |
| `link.file.gid` |  |  |
| `link.file.group` |  |  |
| `link.file.in_upper_layer` |  |  |
| `link.file.inode` |  |  |
| `link.file.mode` |  |  |
| `link.file.mount_id` |  |  |
| `link.file.name` |  |  |
| `link.file.path` |  |  |
| `link.file.rights` |  |  |
| `link.file.uid` |  | uid field definition |
| `link.file.user` |  |  |
| `link.retval` |  |  |

### Event `mkdir`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `mkdir.file.destination.mode` |  |  |
| `mkdir.file.destination.rights` |  |  |
| `mkdir.file.filesystem` |  |  |
| `mkdir.file.gid` |  |  |
| `mkdir.file.group` |  |  |
| `mkdir.file.in_upper_layer` |  |  |
| `mkdir.file.inode` |  |  |
| `mkdir.file.mode` |  |  |
| `mkdir.file.mount_id` |  |  |
| `mkdir.file.name` |  |  |
| `mkdir.file.path` |  |  |
| `mkdir.file.rights` |  |  |
| `mkdir.file.uid` |  | uid field definition |
| `mkdir.file.user` |  |  |
| `mkdir.retval` |  |  |

### Event `open`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `open.file.destination.mode` |  |  |
| `open.file.filesystem` |  |  |
| `open.file.gid` |  |  |
| `open.file.group` |  |  |
| `open.file.in_upper_layer` |  |  |
| `open.file.inode` |  |  |
| `open.file.mode` |  |  |
| `open.file.mount_id` |  |  |
| `open.file.name` |  |  |
| `open.file.path` |  |  |
| `open.file.rights` |  |  |
| `open.file.uid` |  | uid field definition |
| `open.file.user` |  | test traduction |
| `open.flags` |  |  |
| `open.retval` |  |  |

### Event `removexattr`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `removexattr.file.destination.name` |  |  |
| `removexattr.file.destination.namespace` |  |  |
| `removexattr.file.filesystem` |  |  |
| `removexattr.file.gid` |  |  |
| `removexattr.file.group` |  |  |
| `removexattr.file.in_upper_layer` |  |  |
| `removexattr.file.inode` |  |  |
| `removexattr.file.mode` |  |  |
| `removexattr.file.mount_id` |  |  |
| `removexattr.file.name` |  |  |
| `removexattr.file.path` |  |  |
| `removexattr.file.rights` |  |  |
| `removexattr.file.uid` |  | uid field definition |
| `removexattr.file.user` |  |  |
| `removexattr.retval` |  |  |

### Event `rename`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `rename.file.destination.filesystem` |  |  |
| `rename.file.destination.gid` |  |  |
| `rename.file.destination.group` |  |  |
| `rename.file.destination.in_upper_layer` |  |  |
| `rename.file.destination.inode` |  |  |
| `rename.file.destination.mode` |  |  |
| `rename.file.destination.mount_id` |  |  |
| `rename.file.destination.name` |  |  |
| `rename.file.destination.path` |  |  |
| `rename.file.destination.rights` |  |  |
| `rename.file.destination.uid` |  | uid field definition |
| `rename.file.destination.user` |  |  |
| `rename.file.filesystem` |  |  |
| `rename.file.gid` |  |  |
| `rename.file.group` |  |  |
| `rename.file.in_upper_layer` |  |  |
| `rename.file.inode` |  |  |
| `rename.file.mode` |  |  |
| `rename.file.mount_id` |  |  |
| `rename.file.name` |  |  |
| `rename.file.path` |  |  |
| `rename.file.rights` |  |  |
| `rename.file.uid` |  | uid field definition |
| `rename.file.user` |  |  |
| `rename.retval` |  |  |

### Event `rmdir`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `rmdir.file.filesystem` |  |  |
| `rmdir.file.gid` |  |  |
| `rmdir.file.group` |  |  |
| `rmdir.file.in_upper_layer` |  |  |
| `rmdir.file.inode` |  |  |
| `rmdir.file.mode` |  |  |
| `rmdir.file.mount_id` |  |  |
| `rmdir.file.name` |  |  |
| `rmdir.file.path` |  |  |
| `rmdir.file.rights` |  |  |
| `rmdir.file.uid` |  | uid field definition |
| `rmdir.file.user` |  |  |
| `rmdir.retval` |  |  |

### Event `selinux`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `selinux.bool.name` |  |  |
| `selinux.bool.state` |  |  |
| `selinux.bool_commit.state` |  |  |
| `selinux.enforce.status` |  |  |

### Event `setgid`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `setgid.egid` |  |  |
| `setgid.egroup` |  |  |
| `setgid.fsgid` |  |  |
| `setgid.fsgroup` |  |  |
| `setgid.gid` |  |  |
| `setgid.group` |  |  |

### Event `setuid`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `setuid.euid` |  |  |
| `setuid.euser` |  |  |
| `setuid.fsuid` |  |  |
| `setuid.fsuser` |  |  |
| `setuid.uid` |  |  |
| `setuid.user` |  |  |

### Event `setxattr`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `setxattr.file.destination.name` |  |  |
| `setxattr.file.destination.namespace` |  |  |
| `setxattr.file.filesystem` |  |  |
| `setxattr.file.gid` |  |  |
| `setxattr.file.group` |  |  |
| `setxattr.file.in_upper_layer` |  |  |
| `setxattr.file.inode` |  |  |
| `setxattr.file.mode` |  |  |
| `setxattr.file.mount_id` |  |  |
| `setxattr.file.name` |  |  |
| `setxattr.file.path` |  |  |
| `setxattr.file.rights` |  |  |
| `setxattr.file.uid` |  | uid field definition |
| `setxattr.file.user` |  |  |
| `setxattr.retval` |  |  |

### Event `unlink`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `unlink.file.filesystem` |  |  |
| `unlink.file.gid` |  |  |
| `unlink.file.group` |  |  |
| `unlink.file.in_upper_layer` |  |  |
| `unlink.file.inode` |  |  |
| `unlink.file.mode` |  |  |
| `unlink.file.mount_id` |  |  |
| `unlink.file.name` |  |  |
| `unlink.file.path` |  |  |
| `unlink.file.rights` |  |  |
| `unlink.file.uid` |  | uid field definition |
| `unlink.file.user` |  |  |
| `unlink.retval` |  |  |

### Event `utimes`

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `utimes.file.filesystem` |  |  |
| `utimes.file.gid` |  |  |
| `utimes.file.group` |  |  |
| `utimes.file.in_upper_layer` |  |  |
| `utimes.file.inode` |  |  |
| `utimes.file.mode` |  |  |
| `utimes.file.mount_id` |  |  |
| `utimes.file.name` |  |  |
| `utimes.file.path` |  |  |
| `utimes.file.rights` |  |  |
| `utimes.file.uid` |  | uid field definition |
| `utimes.file.user` |  |  |
| `utimes.retval` |  |  |


