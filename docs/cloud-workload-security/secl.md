# SECL Documentation

## Event types

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


## Common to all event types

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `*.container.id` | string | ID of the container |
| `*.container.tags` | string | Tags of the container |
| `*.process.ancestors.cap_effective` | int | Effective capability set of the process |
| `*.process.ancestors.cap_permitted` | int | Permitted capability set of the process |
| `*.process.ancestors.comm` | string | Comm attribute of the process |
| `*.process.ancestors.container.id` | string | Container ID |
| `*.process.ancestors.cookie` | int | Cookie of the process |
| `*.process.ancestors.created_at` | int | Timestamp of the creation of the process |
| `*.process.ancestors.egid` | int | Effective GID of the process |
| `*.process.ancestors.egroup` | string | Effective group of the process |
| `*.process.ancestors.euid` | int | Effective UID of the process |
| `*.process.ancestors.euser` | string | Effective user of the process |
| `*.process.ancestors.file.change_time` | int | Change time of the file |
| `*.process.ancestors.file.filesystem` | string | FileSystem of the process executable |
| `*.process.ancestors.file.gid` | int | GID of the file's owner |
| `*.process.ancestors.file.group` | string | Group of the file's owner |
| `*.process.ancestors.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `*.process.ancestors.file.inode` | int | Inode of the file |
| `*.process.ancestors.file.mode` | int | Mode/rights of the file |
| `*.process.ancestors.file.modification_time` | int | Modification time of the file |
| `*.process.ancestors.file.mount_id` | int | Mount ID of the file |
| `*.process.ancestors.file.name` | string | Basename of the path of the process executable |
| `*.process.ancestors.file.path` | string | Path of the process executable |
| `*.process.ancestors.file.rights` | int | Mode/rights of the file |
| `*.process.ancestors.file.uid` | int | UID of the file's owner |
| `*.process.ancestors.file.user` | string | User of the file's owner |
| `*.process.ancestors.fsgid` | int | FileSystem-gid of the process |
| `*.process.ancestors.fsgroup` | string | FileSystem-group of the process |
| `*.process.ancestors.fsuid` | int | FileSystem-uid of the process |
| `*.process.ancestors.fsuser` | string | FileSystem-user of the process |
| `*.process.ancestors.gid` | int | GID of the process |
| `*.process.ancestors.group` | string | Group of the process |
| `*.process.ancestors.pid` | int | Process ID of the process (also called thread group ID) |
| `*.process.ancestors.ppid` | int | Parent process ID |
| `*.process.ancestors.tid` | int | Thread ID of the thread |
| `*.process.ancestors.tty_name` | string | Name of the TTY associated with the process |
| `*.process.ancestors.uid` | int | UID of the process |
| `*.process.ancestors.user` | string | User of the process |
| `*.process.cap_effective` | int | Effective capability set of the process |
| `*.process.cap_permitted` | int | Permitted capability set of the process |
| `*.process.comm` | string | Comm attribute of the process |
| `*.process.container.id` | string | Container ID |
| `*.process.cookie` | int | Cookie of the process |
| `*.process.created_at` | int | Timestamp of the creation of the process |
| `*.process.egid` | int | Effective GID of the process |
| `*.process.egroup` | string | Effective group of the process |
| `*.process.euid` | int | Effective UID of the process |
| `*.process.euser` | string | Effective user of the process |
| `*.process.file.change_time` | int | Change time of the file |
| `*.process.file.filesystem` | string | FileSystem of the process executable |
| `*.process.file.gid` | int | GID of the file's owner |
| `*.process.file.group` | string | Group of the file's owner |
| `*.process.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `*.process.file.inode` | int | Inode of the file |
| `*.process.file.mode` | int | Mode/rights of the file |
| `*.process.file.modification_time` | int | Modification time of the file |
| `*.process.file.mount_id` | int | Mount ID of the file |
| `*.process.file.name` | string | Basename of the path of the process executable |
| `*.process.file.path` | string | Path of the process executable |
| `*.process.file.rights` | int | Mode/rights of the file |
| `*.process.file.uid` | int | UID of the file's owner |
| `*.process.file.user` | string | User of the file's owner |
| `*.process.fsgid` | int | FileSystem-gid of the process |
| `*.process.fsgroup` | string | FileSystem-group of the process |
| `*.process.fsuid` | int | FileSystem-uid of the process |
| `*.process.fsuser` | string | FileSystem-user of the process |
| `*.process.gid` | int | GID of the process |
| `*.process.group` | string | Group of the process |
| `*.process.pid` | int | Process ID of the process (also called thread group ID) |
| `*.process.ppid` | int | Parent process ID |
| `*.process.tid` | int | Thread ID of the thread |
| `*.process.tty_name` | string | Name of the TTY associated with the process |
| `*.process.uid` | int | UID of the process |
| `*.process.user` | string | User of the process |

## Event `capset`

A process changed its capacity set

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `capset.cap_effective` | int | Effective capability set of the process |
| `capset.cap_permitted` | int | Permitted capability set of the process |

## Event `chmod`

A file’s permissions were changed

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `chmod.file.change_time` | int | Change time of the file |
| `chmod.file.destination.mode` | int | New mode/rights of the chmod-ed file |
| `chmod.file.destination.rights` | int | New mode/rights of the chmod-ed file |
| `chmod.file.filesystem` | string | File's filesystem |
| `chmod.file.gid` | int | GID of the file's owner |
| `chmod.file.group` | string | Group of the file's owner |
| `chmod.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `chmod.file.inode` | int | Inode of the file |
| `chmod.file.mode` | int | Mode/rights of the file |
| `chmod.file.modification_time` | int | Modification time of the file |
| `chmod.file.mount_id` | int | Mount ID of the file |
| `chmod.file.name` | string | File's basename |
| `chmod.file.path` | string | File's path |
| `chmod.file.rights` | int | Mode/rights of the file |
| `chmod.file.uid` | int | UID of the file's owner |
| `chmod.file.user` | string | User of the file's owner |
| `chmod.retval` | int | Return value of the syscall |

## Event `chown`

A file’s owner was changed

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `chown.file.change_time` | int | Change time of the file |
| `chown.file.destination.gid` | int | New GID of the chown-ed file's owner |
| `chown.file.destination.group` | string | New group of the chown-ed file's owner |
| `chown.file.destination.uid` | int | New UID of the chown-ed file's owner |
| `chown.file.destination.user` | string | New user of the chown-ed file's owner |
| `chown.file.filesystem` | string | File's filesystem |
| `chown.file.gid` | int | GID of the file's owner |
| `chown.file.group` | string | Group of the file's owner |
| `chown.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `chown.file.inode` | int | Inode of the file |
| `chown.file.mode` | int | Mode/rights of the file |
| `chown.file.modification_time` | int | Modification time of the file |
| `chown.file.mount_id` | int | Mount ID of the file |
| `chown.file.name` | string | File's basename |
| `chown.file.path` | string | File's path |
| `chown.file.rights` | int | Mode/rights of the file |
| `chown.file.uid` | int | UID of the file's owner |
| `chown.file.user` | string | User of the file's owner |
| `chown.retval` | int | Return value of the syscall |

## Event `exec`

A process was executed or forked

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `exec.args` | string | Arguments of the process (as a string) |
| `exec.args_flags` | string | Arguments of the process (as an array) |
| `exec.args_options` | string | Arguments of the process (as an array) |
| `exec.args_truncated` | bool | Indicator of arguments truncation |
| `exec.argv` | string | Arguments of the process (as an array) |
| `exec.cap_effective` | int | Effective capability set of the process |
| `exec.cap_permitted` | int | Permitted capability set of the process |
| `exec.comm` | string | Comm attribute of the process |
| `exec.container.id` | string | Container ID |
| `exec.cookie` | int | Cookie of the process |
| `exec.created_at` | int | Timestamp of the creation of the process |
| `exec.egid` | int | Effective GID of the process |
| `exec.egroup` | string | Effective group of the process |
| `exec.envs` | string | Environment variables of the process |
| `exec.envs_truncated` | bool | Indicator of environment variables truncation |
| `exec.euid` | int | Effective UID of the process |
| `exec.euser` | string | Effective user of the process |
| `exec.file.change_time` | int | Change time of the file |
| `exec.file.filesystem` | string | FileSystem of the process executable |
| `exec.file.gid` | int | GID of the file's owner |
| `exec.file.group` | string | Group of the file's owner |
| `exec.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `exec.file.inode` | int | Inode of the file |
| `exec.file.mode` | int | Mode/rights of the file |
| `exec.file.modification_time` | int | Modification time of the file |
| `exec.file.mount_id` | int | Mount ID of the file |
| `exec.file.name` | string | Basename of the path of the process executable |
| `exec.file.path` | string | Path of the process executable |
| `exec.file.rights` | int | Mode/rights of the file |
| `exec.file.uid` | int | UID of the file's owner |
| `exec.file.user` | string | User of the file's owner |
| `exec.fsgid` | int | FileSystem-gid of the process |
| `exec.fsgroup` | string | FileSystem-group of the process |
| `exec.fsuid` | int | FileSystem-uid of the process |
| `exec.fsuser` | string | FileSystem-user of the process |
| `exec.gid` | int | GID of the process |
| `exec.group` | string | Group of the process |
| `exec.pid` | int | Process ID of the process (also called thread group ID) |
| `exec.ppid` | int | Parent process ID |
| `exec.tid` | int | Thread ID of the thread |
| `exec.tty_name` | string | Name of the TTY associated with the process |
| `exec.uid` | int | UID of the process |
| `exec.user` | string | User of the process |

## Event `link`

Create a new name/alias for a file

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `link.file.change_time` | int | Change time of the file |
| `link.file.destination.change_time` | int | Change time of the file |
| `link.file.destination.filesystem` | string | File's filesystem |
| `link.file.destination.gid` | int | GID of the file's owner |
| `link.file.destination.group` | string | Group of the file's owner |
| `link.file.destination.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `link.file.destination.inode` | int | Inode of the file |
| `link.file.destination.mode` | int | Mode/rights of the file |
| `link.file.destination.modification_time` | int | Modification time of the file |
| `link.file.destination.mount_id` | int | Mount ID of the file |
| `link.file.destination.name` | string | File's basename |
| `link.file.destination.path` | string | File's path |
| `link.file.destination.rights` | int | Mode/rights of the file |
| `link.file.destination.uid` | int | UID of the file's owner |
| `link.file.destination.user` | string | User of the file's owner |
| `link.file.filesystem` | string | File's filesystem |
| `link.file.gid` | int | GID of the file's owner |
| `link.file.group` | string | Group of the file's owner |
| `link.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `link.file.inode` | int | Inode of the file |
| `link.file.mode` | int | Mode/rights of the file |
| `link.file.modification_time` | int | Modification time of the file |
| `link.file.mount_id` | int | Mount ID of the file |
| `link.file.name` | string | File's basename |
| `link.file.path` | string | File's path |
| `link.file.rights` | int | Mode/rights of the file |
| `link.file.uid` | int | UID of the file's owner |
| `link.file.user` | string | User of the file's owner |
| `link.retval` | int | Return value of the syscall |

## Event `mkdir`

A directory was created

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `mkdir.file.change_time` | int | Change time of the file |
| `mkdir.file.destination.mode` | int | Mode/rights of the new directory |
| `mkdir.file.destination.rights` | int | Mode/rights of the new directory |
| `mkdir.file.filesystem` | string | File's filesystem |
| `mkdir.file.gid` | int | GID of the file's owner |
| `mkdir.file.group` | string | Group of the file's owner |
| `mkdir.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `mkdir.file.inode` | int | Inode of the file |
| `mkdir.file.mode` | int | Mode/rights of the file |
| `mkdir.file.modification_time` | int | Modification time of the file |
| `mkdir.file.mount_id` | int | Mount ID of the file |
| `mkdir.file.name` | string | File's basename |
| `mkdir.file.path` | string | File's path |
| `mkdir.file.rights` | int | Mode/rights of the file |
| `mkdir.file.uid` | int | UID of the file's owner |
| `mkdir.file.user` | string | User of the file's owner |
| `mkdir.retval` | int | Return value of the syscall |

## Event `open`

A file was opened

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `open.file.change_time` | int | Change time of the file |
| `open.file.destination.mode` | int | Mode of the created file |
| `open.file.filesystem` | string | File's filesystem |
| `open.file.gid` | int | GID of the file's owner |
| `open.file.group` | string | Group of the file's owner |
| `open.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `open.file.inode` | int | Inode of the file |
| `open.file.mode` | int | Mode/rights of the file |
| `open.file.modification_time` | int | Modification time of the file |
| `open.file.mount_id` | int | Mount ID of the file |
| `open.file.name` | string | File's basename |
| `open.file.path` | string | File's path |
| `open.file.rights` | int | Mode/rights of the file |
| `open.file.uid` | int | UID of the file's owner |
| `open.file.user` | string | test traduction |
| `open.flags` | int | Flags used when opening the file |
| `open.retval` | int | Return value of the syscall |

## Event `removexattr`

Remove extended attributes

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `removexattr.file.change_time` | int | Change time of the file |
| `removexattr.file.destination.name` | string | Name of the extended attribute |
| `removexattr.file.destination.namespace` | string | Namespace of the extended attribute |
| `removexattr.file.filesystem` | string | File's filesystem |
| `removexattr.file.gid` | int | GID of the file's owner |
| `removexattr.file.group` | string | Group of the file's owner |
| `removexattr.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `removexattr.file.inode` | int | Inode of the file |
| `removexattr.file.mode` | int | Mode/rights of the file |
| `removexattr.file.modification_time` | int | Modification time of the file |
| `removexattr.file.mount_id` | int | Mount ID of the file |
| `removexattr.file.name` | string | File's basename |
| `removexattr.file.path` | string | File's path |
| `removexattr.file.rights` | int | Mode/rights of the file |
| `removexattr.file.uid` | int | UID of the file's owner |
| `removexattr.file.user` | string | User of the file's owner |
| `removexattr.retval` | int | Return value of the syscall |

## Event `rename`

A file/directory was renamed

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `rename.file.change_time` | int | Change time of the file |
| `rename.file.destination.change_time` | int | Change time of the file |
| `rename.file.destination.filesystem` | string | File's filesystem |
| `rename.file.destination.gid` | int | GID of the file's owner |
| `rename.file.destination.group` | string | Group of the file's owner |
| `rename.file.destination.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `rename.file.destination.inode` | int | Inode of the file |
| `rename.file.destination.mode` | int | Mode/rights of the file |
| `rename.file.destination.modification_time` | int | Modification time of the file |
| `rename.file.destination.mount_id` | int | Mount ID of the file |
| `rename.file.destination.name` | string | File's basename |
| `rename.file.destination.path` | string | File's path |
| `rename.file.destination.rights` | int | Mode/rights of the file |
| `rename.file.destination.uid` | int | UID of the file's owner |
| `rename.file.destination.user` | string | User of the file's owner |
| `rename.file.filesystem` | string | File's filesystem |
| `rename.file.gid` | int | GID of the file's owner |
| `rename.file.group` | string | Group of the file's owner |
| `rename.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `rename.file.inode` | int | Inode of the file |
| `rename.file.mode` | int | Mode/rights of the file |
| `rename.file.modification_time` | int | Modification time of the file |
| `rename.file.mount_id` | int | Mount ID of the file |
| `rename.file.name` | string | File's basename |
| `rename.file.path` | string | File's path |
| `rename.file.rights` | int | Mode/rights of the file |
| `rename.file.uid` | int | UID of the file's owner |
| `rename.file.user` | string | User of the file's owner |
| `rename.retval` | int | Return value of the syscall |

## Event `rmdir`

A directory was removed

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `rmdir.file.change_time` | int | Change time of the file |
| `rmdir.file.filesystem` | string | File's filesystem |
| `rmdir.file.gid` | int | GID of the file's owner |
| `rmdir.file.group` | string | Group of the file's owner |
| `rmdir.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `rmdir.file.inode` | int | Inode of the file |
| `rmdir.file.mode` | int | Mode/rights of the file |
| `rmdir.file.modification_time` | int | Modification time of the file |
| `rmdir.file.mount_id` | int | Mount ID of the file |
| `rmdir.file.name` | string | File's basename |
| `rmdir.file.path` | string | File's path |
| `rmdir.file.rights` | int | Mode/rights of the file |
| `rmdir.file.uid` | int | UID of the file's owner |
| `rmdir.file.user` | string | User of the file's owner |
| `rmdir.retval` | int | Return value of the syscall |

## Event `selinux`

An SELinux operation was run

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `selinux.bool.name` | string | SELinux boolean name |
| `selinux.bool.state` | string | SELinux boolean new value |
| `selinux.bool_commit.state` | bool | Indicator of a SELinux boolean commit operation |
| `selinux.enforce.status` | string | SELinux enforcement status (one of "enforcing", "permissive", "disabled"") |

## Event `setgid`

A process changed its effective gid

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `setgid.egid` | int | New effective GID of the process |
| `setgid.egroup` | string | New effective group of the process |
| `setgid.fsgid` | int | New FileSystem GID of the process |
| `setgid.fsgroup` | string | New FileSystem group of the process |
| `setgid.gid` | int | New GID of the process |
| `setgid.group` | string | New group of the process |

## Event `setuid`

A process changed its effective uid

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `setuid.euid` | int | New effective UID of the process |
| `setuid.euser` | string | New effective user of the process |
| `setuid.fsuid` | int | New FileSystem UID of the process |
| `setuid.fsuser` | string | New FileSystem user of the process |
| `setuid.uid` | int | New UID of the process |
| `setuid.user` | string | New user of the process |

## Event `setxattr`

Set exteneded attributes

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `setxattr.file.change_time` | int | Change time of the file |
| `setxattr.file.destination.name` | string | Name of the extended attribute |
| `setxattr.file.destination.namespace` | string | Namespace of the extended attribute |
| `setxattr.file.filesystem` | string | File's filesystem |
| `setxattr.file.gid` | int | GID of the file's owner |
| `setxattr.file.group` | string | Group of the file's owner |
| `setxattr.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `setxattr.file.inode` | int | Inode of the file |
| `setxattr.file.mode` | int | Mode/rights of the file |
| `setxattr.file.modification_time` | int | Modification time of the file |
| `setxattr.file.mount_id` | int | Mount ID of the file |
| `setxattr.file.name` | string | File's basename |
| `setxattr.file.path` | string | File's path |
| `setxattr.file.rights` | int | Mode/rights of the file |
| `setxattr.file.uid` | int | UID of the file's owner |
| `setxattr.file.user` | string | User of the file's owner |
| `setxattr.retval` | int | Return value of the syscall |

## Event `unlink`

A file was deleted

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `unlink.file.change_time` | int | Change time of the file |
| `unlink.file.filesystem` | string | File's filesystem |
| `unlink.file.gid` | int | GID of the file's owner |
| `unlink.file.group` | string | Group of the file's owner |
| `unlink.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `unlink.file.inode` | int | Inode of the file |
| `unlink.file.mode` | int | Mode/rights of the file |
| `unlink.file.modification_time` | int | Modification time of the file |
| `unlink.file.mount_id` | int | Mount ID of the file |
| `unlink.file.name` | string | File's basename |
| `unlink.file.path` | string | File's path |
| `unlink.file.rights` | int | Mode/rights of the file |
| `unlink.file.uid` | int | UID of the file's owner |
| `unlink.file.user` | string | User of the file's owner |
| `unlink.retval` | int | Return value of the syscall |

## Event `utimes`

Change file access/modification times

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `utimes.file.change_time` | int | Change time of the file |
| `utimes.file.filesystem` | string | File's filesystem |
| `utimes.file.gid` | int | GID of the file's owner |
| `utimes.file.group` | string | Group of the file's owner |
| `utimes.file.in_upper_layer` | bool | Indicator of the file layer, in an OverlayFS for example |
| `utimes.file.inode` | int | Inode of the file |
| `utimes.file.mode` | int | Mode/rights of the file |
| `utimes.file.modification_time` | int | Modification time of the file |
| `utimes.file.mount_id` | int | Mount ID of the file |
| `utimes.file.name` | string | File's basename |
| `utimes.file.path` | string | File's path |
| `utimes.file.rights` | int | Mode/rights of the file |
| `utimes.file.uid` | int | UID of the file's owner |
| `utimes.file.user` | string | User of the file's owner |
| `utimes.retval` | int | Return value of the syscall |


