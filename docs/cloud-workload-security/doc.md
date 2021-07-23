# SECL Documentation

### Event types

| SECL Event  | Type | Definition             | Agent Version |
| ---         | ---  | ---                    | ---           |
| *           |      |                        |               |
| capset      |      |                        |               |
| chmod       | File | chmod event definition | 7.27          |
| chown       |      |                        |               |
| exec        |      |                        |               |
| link        |      |                        |               |
| mkdir       |      |                        |               |
| open        |      |                        |               |
| removexattr |      |                        |               |
| rename      |      |                        |               |
| rmdir       |      |                        |               |
| selinux     |      |                        |               |
| setgid      |      |                        |               |
| setuid      |      |                        |               |
| setxattr    |      |                        |               |
| unlink      |      |                        |               |
| utimes      |      |                        |               |


### Common to all event types

| Property                                | Type   | Definition           |
| ---                                     | ---    | ---                  |
| *.container.id                          | string |                      |
| *.container.tags                        | string |                      |
| *.process.ancestors.cap_effective       | int    |                      |
| *.process.ancestors.cap_permitted       | int    |                      |
| *.process.ancestors.comm                | string |                      |
| *.process.ancestors.container.id        | string |                      |
| *.process.ancestors.cookie              | int    |                      |
| *.process.ancestors.created_at          | int    |                      |
| *.process.ancestors.egid                | int    |                      |
| *.process.ancestors.egroup              | string |                      |
| *.process.ancestors.euid                | int    |                      |
| *.process.ancestors.euser               | string |                      |
| *.process.ancestors.file.filesystem     | string |                      |
| *.process.ancestors.file.gid            | int    |                      |
| *.process.ancestors.file.group          | string |                      |
| *.process.ancestors.file.in_upper_layer | bool   |                      |
| *.process.ancestors.file.inode          | int    |                      |
| *.process.ancestors.file.mode           | int    |                      |
| *.process.ancestors.file.mount_id       | int    |                      |
| *.process.ancestors.file.name           | string |                      |
| *.process.ancestors.file.path           | string |                      |
| *.process.ancestors.file.rights         | int    |                      |
| *.process.ancestors.file.uid            | int    | uid field definition |
| *.process.ancestors.file.user           | string |                      |
| *.process.ancestors.fsgid               | int    |                      |
| *.process.ancestors.fsgroup             | string |                      |
| *.process.ancestors.fsuid               | int    |                      |
| *.process.ancestors.fsuser              | string |                      |
| *.process.ancestors.gid                 | int    |                      |
| *.process.ancestors.group               | string |                      |
| *.process.ancestors.pid                 | int    |                      |
| *.process.ancestors.ppid                | int    |                      |
| *.process.ancestors.tid                 | int    |                      |
| *.process.ancestors.tty_name            | string |                      |
| *.process.ancestors.uid                 | int    |                      |
| *.process.ancestors.user                | string |                      |
| *.process.cap_effective                 | int    |                      |
| *.process.cap_permitted                 | int    |                      |
| *.process.comm                          | string |                      |
| *.process.container.id                  | string |                      |
| *.process.cookie                        | int    |                      |
| *.process.created_at                    | int    |                      |
| *.process.egid                          | int    |                      |
| *.process.egroup                        | string |                      |
| *.process.euid                          | int    |                      |
| *.process.euser                         | string |                      |
| *.process.file.filesystem               | string |                      |
| *.process.file.gid                      | int    |                      |
| *.process.file.group                    | string |                      |
| *.process.file.in_upper_layer           | bool   |                      |
| *.process.file.inode                    | int    |                      |
| *.process.file.mode                     | int    |                      |
| *.process.file.mount_id                 | int    |                      |
| *.process.file.name                     | string |                      |
| *.process.file.path                     | string |                      |
| *.process.file.rights                   | int    |                      |
| *.process.file.uid                      | int    | uid field definition |
| *.process.file.user                     | string |                      |
| *.process.fsgid                         | int    |                      |
| *.process.fsgroup                       | string |                      |
| *.process.fsuid                         | int    |                      |
| *.process.fsuser                        | string |                      |
| *.process.gid                           | int    |                      |
| *.process.group                         | string |                      |
| *.process.pid                           | int    |                      |
| *.process.ppid                          | int    |                      |
| *.process.tid                           | int    |                      |
| *.process.tty_name                      | string |                      |
| *.process.uid                           | int    |                      |
| *.process.user                          | string |                      |


### Event `capset`

| Property             | Type | Definition |
| ---                  | ---  | ---        |
| capset.cap_effective | int  |            |
| capset.cap_permitted | int  |            |


### Event `chmod`

| Property                      | Type   | Definition           |
| ---                           | ---    | ---                  |
| chmod.file.destination.mode   | int    |                      |
| chmod.file.destination.rights | int    |                      |
| chmod.file.filesystem         | string |                      |
| chmod.file.gid                | int    |                      |
| chmod.file.group              | string |                      |
| chmod.file.in_upper_layer     | bool   |                      |
| chmod.file.inode              | int    |                      |
| chmod.file.mode               | int    |                      |
| chmod.file.mount_id           | int    |                      |
| chmod.file.name               | string |                      |
| chmod.file.path               | string |                      |
| chmod.file.rights             | int    |                      |
| chmod.file.uid                | int    | uid field definition |
| chmod.file.user               | string |                      |
| chmod.retval                  | int    |                      |


### Event `chown`

| Property                     | Type   | Definition           |
| ---                          | ---    | ---                  |
| chown.file.destination.gid   | int    |                      |
| chown.file.destination.group | string |                      |
| chown.file.destination.uid   | int    |                      |
| chown.file.destination.user  | string |                      |
| chown.file.filesystem        | string |                      |
| chown.file.gid               | int    |                      |
| chown.file.group             | string |                      |
| chown.file.in_upper_layer    | bool   |                      |
| chown.file.inode             | int    |                      |
| chown.file.mode              | int    |                      |
| chown.file.mount_id          | int    |                      |
| chown.file.name              | string |                      |
| chown.file.path              | string |                      |
| chown.file.rights            | int    |                      |
| chown.file.uid               | int    | uid field definition |
| chown.file.user              | string |                      |
| chown.retval                 | int    |                      |


### Event `exec`

| Property                 | Type   | Definition           |
| ---                      | ---    | ---                  |
| exec.args                | string |                      |
| exec.args_flags          | string |                      |
| exec.args_options        | string |                      |
| exec.args_truncated      | bool   |                      |
| exec.argv                | string |                      |
| exec.cap_effective       | int    |                      |
| exec.cap_permitted       | int    |                      |
| exec.comm                | string |                      |
| exec.container.id        | string |                      |
| exec.cookie              | int    |                      |
| exec.created_at          | int    |                      |
| exec.egid                | int    |                      |
| exec.egroup              | string |                      |
| exec.envs                | string |                      |
| exec.envs_truncated      | bool   |                      |
| exec.euid                | int    |                      |
| exec.euser               | string |                      |
| exec.file.filesystem     | string |                      |
| exec.file.gid            | int    |                      |
| exec.file.group          | string |                      |
| exec.file.in_upper_layer | bool   |                      |
| exec.file.inode          | int    |                      |
| exec.file.mode           | int    |                      |
| exec.file.mount_id       | int    |                      |
| exec.file.name           | string |                      |
| exec.file.path           | string |                      |
| exec.file.rights         | int    |                      |
| exec.file.uid            | int    | uid field definition |
| exec.file.user           | string |                      |
| exec.fsgid               | int    |                      |
| exec.fsgroup             | string |                      |
| exec.fsuid               | int    |                      |
| exec.fsuser              | string |                      |
| exec.gid                 | int    |                      |
| exec.group               | string |                      |
| exec.pid                 | int    |                      |
| exec.ppid                | int    |                      |
| exec.tid                 | int    |                      |
| exec.tty_name            | string |                      |
| exec.uid                 | int    |                      |
| exec.user                | string |                      |


### Event `link`

| Property                             | Type   | Definition           |
| ---                                  | ---    | ---                  |
| link.file.destination.filesystem     | string |                      |
| link.file.destination.gid            | int    |                      |
| link.file.destination.group          | string |                      |
| link.file.destination.in_upper_layer | bool   |                      |
| link.file.destination.inode          | int    |                      |
| link.file.destination.mode           | int    |                      |
| link.file.destination.mount_id       | int    |                      |
| link.file.destination.name           | string |                      |
| link.file.destination.path           | string |                      |
| link.file.destination.rights         | int    |                      |
| link.file.destination.uid            | int    | uid field definition |
| link.file.destination.user           | string |                      |
| link.file.filesystem                 | string |                      |
| link.file.gid                        | int    |                      |
| link.file.group                      | string |                      |
| link.file.in_upper_layer             | bool   |                      |
| link.file.inode                      | int    |                      |
| link.file.mode                       | int    |                      |
| link.file.mount_id                   | int    |                      |
| link.file.name                       | string |                      |
| link.file.path                       | string |                      |
| link.file.rights                     | int    |                      |
| link.file.uid                        | int    | uid field definition |
| link.file.user                       | string |                      |
| link.retval                          | int    |                      |


### Event `mkdir`

| Property                      | Type   | Definition           |
| ---                           | ---    | ---                  |
| mkdir.file.destination.mode   | int    |                      |
| mkdir.file.destination.rights | int    |                      |
| mkdir.file.filesystem         | string |                      |
| mkdir.file.gid                | int    |                      |
| mkdir.file.group              | string |                      |
| mkdir.file.in_upper_layer     | bool   |                      |
| mkdir.file.inode              | int    |                      |
| mkdir.file.mode               | int    |                      |
| mkdir.file.mount_id           | int    |                      |
| mkdir.file.name               | string |                      |
| mkdir.file.path               | string |                      |
| mkdir.file.rights             | int    |                      |
| mkdir.file.uid                | int    | uid field definition |
| mkdir.file.user               | string |                      |
| mkdir.retval                  | int    |                      |


### Event `open`

| Property                   | Type   | Definition           |
| ---                        | ---    | ---                  |
| open.file.destination.mode | int    |                      |
| open.file.filesystem       | string |                      |
| open.file.gid              | int    |                      |
| open.file.group            | string |                      |
| open.file.in_upper_layer   | bool   |                      |
| open.file.inode            | int    |                      |
| open.file.mode             | int    |                      |
| open.file.mount_id         | int    |                      |
| open.file.name             | string |                      |
| open.file.path             | string |                      |
| open.file.rights           | int    |                      |
| open.file.uid              | int    | uid field definition |
| open.file.user             | string | test traduction      |
| open.flags                 | int    |                      |
| open.retval                | int    |                      |


### Event `removexattr`

| Property                               | Type   | Definition           |
| ---                                    | ---    | ---                  |
| removexattr.file.destination.name      | string |                      |
| removexattr.file.destination.namespace | string |                      |
| removexattr.file.filesystem            | string |                      |
| removexattr.file.gid                   | int    |                      |
| removexattr.file.group                 | string |                      |
| removexattr.file.in_upper_layer        | bool   |                      |
| removexattr.file.inode                 | int    |                      |
| removexattr.file.mode                  | int    |                      |
| removexattr.file.mount_id              | int    |                      |
| removexattr.file.name                  | string |                      |
| removexattr.file.path                  | string |                      |
| removexattr.file.rights                | int    |                      |
| removexattr.file.uid                   | int    | uid field definition |
| removexattr.file.user                  | string |                      |
| removexattr.retval                     | int    |                      |


### Event `rename`

| Property                               | Type   | Definition           |
| ---                                    | ---    | ---                  |
| rename.file.destination.filesystem     | string |                      |
| rename.file.destination.gid            | int    |                      |
| rename.file.destination.group          | string |                      |
| rename.file.destination.in_upper_layer | bool   |                      |
| rename.file.destination.inode          | int    |                      |
| rename.file.destination.mode           | int    |                      |
| rename.file.destination.mount_id       | int    |                      |
| rename.file.destination.name           | string |                      |
| rename.file.destination.path           | string |                      |
| rename.file.destination.rights         | int    |                      |
| rename.file.destination.uid            | int    | uid field definition |
| rename.file.destination.user           | string |                      |
| rename.file.filesystem                 | string |                      |
| rename.file.gid                        | int    |                      |
| rename.file.group                      | string |                      |
| rename.file.in_upper_layer             | bool   |                      |
| rename.file.inode                      | int    |                      |
| rename.file.mode                       | int    |                      |
| rename.file.mount_id                   | int    |                      |
| rename.file.name                       | string |                      |
| rename.file.path                       | string |                      |
| rename.file.rights                     | int    |                      |
| rename.file.uid                        | int    | uid field definition |
| rename.file.user                       | string |                      |
| rename.retval                          | int    |                      |


### Event `rmdir`

| Property                  | Type   | Definition           |
| ---                       | ---    | ---                  |
| rmdir.file.filesystem     | string |                      |
| rmdir.file.gid            | int    |                      |
| rmdir.file.group          | string |                      |
| rmdir.file.in_upper_layer | bool   |                      |
| rmdir.file.inode          | int    |                      |
| rmdir.file.mode           | int    |                      |
| rmdir.file.mount_id       | int    |                      |
| rmdir.file.name           | string |                      |
| rmdir.file.path           | string |                      |
| rmdir.file.rights         | int    |                      |
| rmdir.file.uid            | int    | uid field definition |
| rmdir.file.user           | string |                      |
| rmdir.retval              | int    |                      |


### Event `selinux`

| Property                  | Type   | Definition |
| ---                       | ---    | ---        |
| selinux.bool.name         | string |            |
| selinux.bool.state        | string |            |
| selinux.bool_commit.state | bool   |            |
| selinux.enforce.status    | string |            |


### Event `setgid`

| Property       | Type   | Definition |
| ---            | ---    | ---        |
| setgid.egid    | int    |            |
| setgid.egroup  | string |            |
| setgid.fsgid   | int    |            |
| setgid.fsgroup | string |            |
| setgid.gid     | int    |            |
| setgid.group   | string |            |


### Event `setuid`

| Property      | Type   | Definition |
| ---           | ---    | ---        |
| setuid.euid   | int    |            |
| setuid.euser  | string |            |
| setuid.fsuid  | int    |            |
| setuid.fsuser | string |            |
| setuid.uid    | int    |            |
| setuid.user   | string |            |


### Event `setxattr`

| Property                            | Type   | Definition           |
| ---                                 | ---    | ---                  |
| setxattr.file.destination.name      | string |                      |
| setxattr.file.destination.namespace | string |                      |
| setxattr.file.filesystem            | string |                      |
| setxattr.file.gid                   | int    |                      |
| setxattr.file.group                 | string |                      |
| setxattr.file.in_upper_layer        | bool   |                      |
| setxattr.file.inode                 | int    |                      |
| setxattr.file.mode                  | int    |                      |
| setxattr.file.mount_id              | int    |                      |
| setxattr.file.name                  | string |                      |
| setxattr.file.path                  | string |                      |
| setxattr.file.rights                | int    |                      |
| setxattr.file.uid                   | int    | uid field definition |
| setxattr.file.user                  | string |                      |
| setxattr.retval                     | int    |                      |


### Event `unlink`

| Property                   | Type   | Definition           |
| ---                        | ---    | ---                  |
| unlink.file.filesystem     | string |                      |
| unlink.file.gid            | int    |                      |
| unlink.file.group          | string |                      |
| unlink.file.in_upper_layer | bool   |                      |
| unlink.file.inode          | int    |                      |
| unlink.file.mode           | int    |                      |
| unlink.file.mount_id       | int    |                      |
| unlink.file.name           | string |                      |
| unlink.file.path           | string |                      |
| unlink.file.rights         | int    |                      |
| unlink.file.uid            | int    | uid field definition |
| unlink.file.user           | string |                      |
| unlink.retval              | int    |                      |


### Event `utimes`

| Property                   | Type   | Definition           |
| ---                        | ---    | ---                  |
| utimes.file.filesystem     | string |                      |
| utimes.file.gid            | int    |                      |
| utimes.file.group          | string |                      |
| utimes.file.in_upper_layer | bool   |                      |
| utimes.file.inode          | int    |                      |
| utimes.file.mode           | int    |                      |
| utimes.file.mount_id       | int    |                      |
| utimes.file.name           | string |                      |
| utimes.file.path           | string |                      |
| utimes.file.rights         | int    |                      |
| utimes.file.uid            | int    | uid field definition |
| utimes.file.user           | string |                      |
| utimes.retval              | int    |                      |


