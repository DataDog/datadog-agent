# CWS Event Documentation

The CWS event sent to the backend by the security agent respects the following schema:
```
BACKEND_EVENT_SCHEMA = {
    "properties": {
        "evt": {
            "$ref": "#/definitions/EventContext"
        },
        "file": {
            "$ref": "#/definitions/FileEvent"
        },
        "selinux": {
            "$ref": "#/definitions/SELinuxEvent"
        },
        "usr": {
            "$ref": "#/definitions/UserContext"
        },
        "process": {
            "$ref": "#/definitions/ProcessContext"
        },
        "container": {
            "$ref": "#/definitions/ContainerContext"
        },
        "date": {
            "type": "string",
            "format": "date-time"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| `evt` | $ref | Please see [EventContext](#eventcontext) |
| `file` | $ref | Please see [FileEvent](#fileevent) |
| `selinux` | $ref | Please see [SELinuxEvent](#selinuxevent) |
| `usr` | $ref | Please see [UserContext](#usercontext) |
| `process` | $ref | Please see [ProcessContext](#processcontext) |
| `container` | $ref | Please see [ContainerContext](#containercontext) |
| `date` | string |  |

## `ContainerContext`

```
{
    "properties": {
        "id": {
            "type": "string",
            "description": "Container ID"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `id` | Container ID |


## `EventContext`

```
{
    "properties": {
        "name": {
            "type": "string",
            "description": "Event name"
        },
        "category": {
            "type": "string",
            "description": "Event category"
        },
        "outcome": {
            "type": "string",
            "description": "Event outcome"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `name` | Event name |
| `category` | Event category |
| `outcome` | Event outcome |


## `File`

```
{
    "required": [
        "uid",
        "gid"
    ],
    "properties": {
        "path": {
            "type": "string",
            "description": "File path"
        },
        "name": {
            "type": "string",
            "description": "File basename"
        },
        "path_resolution_error": {
            "type": "string",
            "description": "Error message from path resolution"
        },
        "inode": {
            "type": "integer",
            "description": "File inode number"
        },
        "mode": {
            "type": "integer",
            "description": "File mode"
        },
        "in_upper_layer": {
            "type": "boolean",
            "description": "Indicator of file OverlayFS layer"
        },
        "mount_id": {
            "type": "integer",
            "description": "File mount ID"
        },
        "filesystem": {
            "type": "string",
            "description": "File filesystem name"
        },
        "uid": {
            "type": "integer",
            "description": "File User ID"
        },
        "gid": {
            "type": "integer",
            "description": "File Group ID"
        },
        "user": {
            "type": "string",
            "description": "File user"
        },
        "group": {
            "type": "string",
            "description": "File group"
        },
        "attribute_name": {
            "type": "string",
            "description": "File extended attribute name"
        },
        "attribute_namespace": {
            "type": "string",
            "description": "File extended attribute namespace"
        },
        "flags": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "File flags"
        },
        "access_time": {
            "type": "string",
            "format": "date-time"
        },
        "modification_time": {
            "type": "string",
            "description": "File modified time",
            "format": "date-time"
        },
        "change_time": {
            "type": "string",
            "description": "File change time",
            "format": "date-time"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `path` | File path |
| `name` | File basename |
| `path_resolution_error` | Error message from path resolution |
| `inode` | File inode number |
| `mode` | File mode |
| `in_upper_layer` | Indicator of file OverlayFS layer |
| `mount_id` | File mount ID |
| `filesystem` | File filesystem name |
| `uid` | File User ID |
| `gid` | File Group ID |
| `user` | File user |
| `group` | File group |
| `attribute_name` | File extended attribute name |
| `attribute_namespace` | File extended attribute namespace |
| `flags` | File flags |
| `modification_time` | File modified time |
| `change_time` | File change time |


## `FileEvent`

```
{
    "required": [
        "uid",
        "gid"
    ],
    "properties": {
        "path": {
            "type": "string",
            "description": "File path"
        },
        "name": {
            "type": "string",
            "description": "File basename"
        },
        "path_resolution_error": {
            "type": "string",
            "description": "Error message from path resolution"
        },
        "inode": {
            "type": "integer",
            "description": "File inode number"
        },
        "mode": {
            "type": "integer",
            "description": "File mode"
        },
        "in_upper_layer": {
            "type": "boolean",
            "description": "Indicator of file OverlayFS layer"
        },
        "mount_id": {
            "type": "integer",
            "description": "File mount ID"
        },
        "filesystem": {
            "type": "string",
            "description": "File filesystem name"
        },
        "uid": {
            "type": "integer",
            "description": "File User ID"
        },
        "gid": {
            "type": "integer",
            "description": "File Group ID"
        },
        "user": {
            "type": "string",
            "description": "File user"
        },
        "group": {
            "type": "string",
            "description": "File group"
        },
        "attribute_name": {
            "type": "string",
            "description": "File extended attribute name"
        },
        "attribute_namespace": {
            "type": "string",
            "description": "File extended attribute namespace"
        },
        "flags": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "File flags"
        },
        "access_time": {
            "type": "string",
            "format": "date-time"
        },
        "modification_time": {
            "type": "string",
            "description": "File modified time",
            "format": "date-time"
        },
        "change_time": {
            "type": "string",
            "description": "File change time",
            "format": "date-time"
        },
        "destination": {
            "$ref": "#/definitions/File",
            "description": "Target file information"
        },
        "new_mount_id": {
            "type": "integer",
            "description": "New Mount ID"
        },
        "group_id": {
            "type": "integer",
            "description": "Group ID"
        },
        "device": {
            "type": "integer",
            "description": "Device associated with the file"
        },
        "fstype": {
            "type": "string",
            "description": "Filesystem type"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `path` | File path |
| `name` | File basename |
| `path_resolution_error` | Error message from path resolution |
| `inode` | File inode number |
| `mode` | File mode |
| `in_upper_layer` | Indicator of file OverlayFS layer |
| `mount_id` | File mount ID |
| `filesystem` | File filesystem name |
| `uid` | File User ID |
| `gid` | File Group ID |
| `user` | File user |
| `group` | File group |
| `attribute_name` | File extended attribute name |
| `attribute_namespace` | File extended attribute namespace |
| `flags` | File flags |
| `modification_time` | File modified time |
| `change_time` | File change time |
| `destination` | Target file information |
| `new_mount_id` | New Mount ID |
| `group_id` | Group ID |
| `device` | Device associated with the file |
| `fstype` | Filesystem type |

| References |
| ---------- |
| [File](#file) |

## `ProcessCacheEntry`

```
{
    "required": [
        "uid",
        "gid"
    ],
    "properties": {
        "pid": {
            "type": "integer",
            "description": "Process ID"
        },
        "ppid": {
            "type": "integer",
            "description": "Parent Process ID"
        },
        "tid": {
            "type": "integer",
            "description": "Thread ID"
        },
        "uid": {
            "type": "integer",
            "description": "User ID"
        },
        "gid": {
            "type": "integer",
            "description": "Group ID"
        },
        "user": {
            "type": "string",
            "description": "User name"
        },
        "group": {
            "type": "string",
            "description": "Group name"
        },
        "path_resolution_error": {
            "type": "string",
            "description": "Description of an error in the path resolution"
        },
        "comm": {
            "type": "string",
            "description": "Command name"
        },
        "tty": {
            "type": "string",
            "description": "TTY associated with the process"
        },
        "fork_time": {
            "type": "string",
            "description": "Fork time of the process",
            "format": "date-time"
        },
        "exec_time": {
            "type": "string",
            "description": "Exec time of the process",
            "format": "date-time"
        },
        "exit_time": {
            "type": "string",
            "description": "Exit time of the process",
            "format": "date-time"
        },
        "credentials": {
            "$ref": "#/definitions/ProcessCredentials",
            "description": "Credentials associated with the process"
        },
        "executable": {
            "$ref": "#/definitions/File",
            "description": "File information of the executable"
        },
        "container": {
            "$ref": "#/definitions/ContainerContext",
            "description": "Container context"
        },
        "args": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "Command line arguments"
        },
        "args_truncated": {
            "type": "boolean",
            "description": "Indicator of arguments truncation"
        },
        "envs": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "Environment variables of the process"
        },
        "envs_truncated": {
            "type": "boolean",
            "description": "Indicator of environments variable truncation"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `pid` | Process ID |
| `ppid` | Parent Process ID |
| `tid` | Thread ID |
| `uid` | User ID |
| `gid` | Group ID |
| `user` | User name |
| `group` | Group name |
| `path_resolution_error` | Description of an error in the path resolution |
| `comm` | Command name |
| `tty` | TTY associated with the process |
| `fork_time` | Fork time of the process |
| `exec_time` | Exec time of the process |
| `exit_time` | Exit time of the process |
| `credentials` | Credentials associated with the process |
| `executable` | File information of the executable |
| `container` | Container context |
| `args` | Command line arguments |
| `args_truncated` | Indicator of arguments truncation |
| `envs` | Environment variables of the process |
| `envs_truncated` | Indicator of environments variable truncation |

| References |
| ---------- |
| [ProcessCredentials](#processcredentials) |
| [File](#file) |
| [ContainerContext](#containercontext) |

## `ProcessContext`

```
{
    "required": [
        "uid",
        "gid"
    ],
    "properties": {
        "pid": {
            "type": "integer",
            "description": "Process ID"
        },
        "ppid": {
            "type": "integer",
            "description": "Parent Process ID"
        },
        "tid": {
            "type": "integer",
            "description": "Thread ID"
        },
        "uid": {
            "type": "integer",
            "description": "User ID"
        },
        "gid": {
            "type": "integer",
            "description": "Group ID"
        },
        "user": {
            "type": "string",
            "description": "User name"
        },
        "group": {
            "type": "string",
            "description": "Group name"
        },
        "path_resolution_error": {
            "type": "string",
            "description": "Description of an error in the path resolution"
        },
        "comm": {
            "type": "string",
            "description": "Command name"
        },
        "tty": {
            "type": "string",
            "description": "TTY associated with the process"
        },
        "fork_time": {
            "type": "string",
            "description": "Fork time of the process",
            "format": "date-time"
        },
        "exec_time": {
            "type": "string",
            "description": "Exec time of the process",
            "format": "date-time"
        },
        "exit_time": {
            "type": "string",
            "description": "Exit time of the process",
            "format": "date-time"
        },
        "credentials": {
            "$ref": "#/definitions/ProcessCredentials",
            "description": "Credentials associated with the process"
        },
        "executable": {
            "$ref": "#/definitions/File",
            "description": "File information of the executable"
        },
        "container": {
            "$ref": "#/definitions/ContainerContext",
            "description": "Container context"
        },
        "args": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "Command line arguments"
        },
        "args_truncated": {
            "type": "boolean",
            "description": "Indicator of arguments truncation"
        },
        "envs": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "Environment variables of the process"
        },
        "envs_truncated": {
            "type": "boolean",
            "description": "Indicator of environments variable truncation"
        },
        "parent": {
            "$ref": "#/definitions/ProcessCacheEntry",
            "description": "Parent process"
        },
        "ancestors": {
            "items": {
                "$ref": "#/definitions/ProcessCacheEntry"
            },
            "type": "array",
            "description": "Ancestor processes"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `pid` | Process ID |
| `ppid` | Parent Process ID |
| `tid` | Thread ID |
| `uid` | User ID |
| `gid` | Group ID |
| `user` | User name |
| `group` | Group name |
| `path_resolution_error` | Description of an error in the path resolution |
| `comm` | Command name |
| `tty` | TTY associated with the process |
| `fork_time` | Fork time of the process |
| `exec_time` | Exec time of the process |
| `exit_time` | Exit time of the process |
| `credentials` | Credentials associated with the process |
| `executable` | File information of the executable |
| `container` | Container context |
| `args` | Command line arguments |
| `args_truncated` | Indicator of arguments truncation |
| `envs` | Environment variables of the process |
| `envs_truncated` | Indicator of environments variable truncation |
| `parent` | Parent process |
| `ancestors` | Ancestor processes |

| References |
| ---------- |
| [ProcessCredentials](#processcredentials) |
| [File](#file) |
| [ContainerContext](#containercontext) |
| [ProcessCacheEntry](#processcacheentry) |

## `ProcessCredentials`

```
{
    "required": [
        "uid",
        "gid",
        "euid",
        "egid",
        "fsuid",
        "fsgid",
        "cap_effective",
        "cap_permitted"
    ],
    "properties": {
        "uid": {
            "type": "integer",
            "description": "User ID"
        },
        "user": {
            "type": "string",
            "description": "User name"
        },
        "gid": {
            "type": "integer",
            "description": "Group ID"
        },
        "group": {
            "type": "string",
            "description": "Group name"
        },
        "euid": {
            "type": "integer",
            "description": "Effective User ID"
        },
        "euser": {
            "type": "string",
            "description": "Effective User name"
        },
        "egid": {
            "type": "integer",
            "description": "Effective Group ID"
        },
        "egroup": {
            "type": "string",
            "description": "Effective Group name"
        },
        "fsuid": {
            "type": "integer",
            "description": "Filesystem User ID"
        },
        "fsuser": {
            "type": "string",
            "description": "Filesystem User name"
        },
        "fsgid": {
            "type": "integer",
            "description": "Filesystem Group ID"
        },
        "fsgroup": {
            "type": "string",
            "description": "Filesystem Group name"
        },
        "cap_effective": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "Effective Capacity set"
        },
        "cap_permitted": {
            "items": {
                "type": "string"
            },
            "type": "array",
            "description": "Permitted Capacity set"
        },
        "destination": {
            "additionalProperties": true,
            "description": "Credentials after the operation"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `uid` | User ID |
| `user` | User name |
| `gid` | Group ID |
| `group` | Group name |
| `euid` | Effective User ID |
| `euser` | Effective User name |
| `egid` | Effective Group ID |
| `egroup` | Effective Group name |
| `fsuid` | Filesystem User ID |
| `fsuser` | Filesystem User name |
| `fsgid` | Filesystem Group ID |
| `fsgroup` | Filesystem Group name |
| `cap_effective` | Effective Capacity set |
| `cap_permitted` | Permitted Capacity set |
| `destination` | Credentials after the operation |


## `SELinuxBoolChange`

```
{
    "properties": {
        "name": {
            "type": "string",
            "description": "SELinux boolean name"
        },
        "state": {
            "type": "string",
            "description": "SELinux boolean state ('on' or 'off')"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `name` | SELinux boolean name |
| `state` | SELinux boolean state ('on' or 'off') |


## `SELinuxBoolCommit`

```
{
    "properties": {
        "state": {
            "type": "boolean",
            "description": "SELinux boolean commit operation"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `state` | SELinux boolean commit operation |


## `SELinuxEnforceStatus`

```
{
    "properties": {
        "status": {
            "type": "string",
            "description": "SELinux enforcement status (one of 'enforcing', 'permissive' or 'disabled')"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `status` | SELinux enforcement status (one of 'enforcing', 'permissive' or 'disabled') |


## `SELinuxEvent`

```
{
    "properties": {
        "bool": {
            "$ref": "#/definitions/SELinuxBoolChange",
            "description": "SELinux boolean operation"
        },
        "enforce": {
            "$ref": "#/definitions/SELinuxEnforceStatus",
            "description": "SELinux enforcement change"
        },
        "bool_commit": {
            "$ref": "#/definitions/SELinuxBoolCommit",
            "description": "SELinux boolean commit"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `bool` | SELinux boolean operation |
| `enforce` | SELinux enforcement change |
| `bool_commit` | SELinux boolean commit |

| References |
| ---------- |
| [SELinuxBoolChange](#selinuxboolchange) |
| [SELinuxEnforceStatus](#selinuxenforcestatus) |
| [SELinuxBoolCommit](#selinuxboolcommit) |

## `UserContext`

```
{
    "properties": {
        "id": {
            "type": "string",
            "description": "User name"
        },
        "group": {
            "type": "string",
            "description": "Group name"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```

| Field | Description |
| ----- | ----------- |
| `id` | User name |
| `group` | Group name |



