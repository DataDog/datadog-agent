# Backend event Documentation

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
            "type": "string"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```



## `EventContext`

```
{
    "properties": {
        "name": {
            "type": "string"
        },
        "category": {
            "type": "string"
        },
        "outcome": {
            "type": "string"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```



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
            "$ref": "#/definitions/File"
        },
        "new_mount_id": {
            "type": "integer"
        },
        "group_id": {
            "type": "integer"
        },
        "device": {
            "type": "integer"
        },
        "fstype": {
            "type": "string"
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
            "type": "integer"
        },
        "ppid": {
            "type": "integer"
        },
        "tid": {
            "type": "integer"
        },
        "uid": {
            "type": "integer"
        },
        "gid": {
            "type": "integer"
        },
        "user": {
            "type": "string"
        },
        "group": {
            "type": "string"
        },
        "executable_path": {
            "type": "string"
        },
        "path_resolution_error": {
            "type": "string"
        },
        "comm": {
            "type": "string"
        },
        "executable_inode": {
            "type": "integer"
        },
        "executable_mount_id": {
            "type": "integer"
        },
        "executable_filesystem": {
            "type": "string"
        },
        "tty": {
            "type": "string"
        },
        "fork_time": {
            "type": "string",
            "format": "date-time"
        },
        "exec_time": {
            "type": "string",
            "format": "date-time"
        },
        "exit_time": {
            "type": "string",
            "format": "date-time"
        },
        "credentials": {
            "$ref": "#/definitions/ProcessCredentials"
        },
        "executable": {
            "$ref": "#/definitions/File"
        },
        "container": {
            "$ref": "#/definitions/ContainerContext"
        },
        "args": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "args_truncated": {
            "type": "boolean"
        },
        "envs": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "envs_truncated": {
            "type": "boolean"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```


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
            "type": "integer"
        },
        "ppid": {
            "type": "integer"
        },
        "tid": {
            "type": "integer"
        },
        "uid": {
            "type": "integer"
        },
        "gid": {
            "type": "integer"
        },
        "user": {
            "type": "string"
        },
        "group": {
            "type": "string"
        },
        "executable_path": {
            "type": "string"
        },
        "path_resolution_error": {
            "type": "string"
        },
        "comm": {
            "type": "string"
        },
        "executable_inode": {
            "type": "integer"
        },
        "executable_mount_id": {
            "type": "integer"
        },
        "executable_filesystem": {
            "type": "string"
        },
        "tty": {
            "type": "string"
        },
        "fork_time": {
            "type": "string",
            "format": "date-time"
        },
        "exec_time": {
            "type": "string",
            "format": "date-time"
        },
        "exit_time": {
            "type": "string",
            "format": "date-time"
        },
        "credentials": {
            "$ref": "#/definitions/ProcessCredentials"
        },
        "executable": {
            "$ref": "#/definitions/File"
        },
        "container": {
            "$ref": "#/definitions/ContainerContext"
        },
        "args": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "args_truncated": {
            "type": "boolean"
        },
        "envs": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "envs_truncated": {
            "type": "boolean"
        },
        "parent": {
            "$ref": "#/definitions/ProcessCacheEntry"
        },
        "ancestors": {
            "items": {
                "$ref": "#/definitions/ProcessCacheEntry"
            },
            "type": "array"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```


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
            "type": "integer"
        },
        "user": {
            "type": "string"
        },
        "gid": {
            "type": "integer"
        },
        "group": {
            "type": "string"
        },
        "euid": {
            "type": "integer"
        },
        "euser": {
            "type": "string"
        },
        "egid": {
            "type": "integer"
        },
        "egroup": {
            "type": "string"
        },
        "fsuid": {
            "type": "integer"
        },
        "fsuser": {
            "type": "string"
        },
        "fsgid": {
            "type": "integer"
        },
        "fsgroup": {
            "type": "string"
        },
        "cap_effective": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "cap_permitted": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "destination": {
            "additionalProperties": true
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```



## `SELinuxBoolChange`

```
{
    "properties": {
        "name": {
            "type": "string"
        },
        "state": {
            "type": "string"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```



## `SELinuxBoolCommit`

```
{
    "properties": {
        "state": {
            "type": "boolean"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```



## `SELinuxEnforceStatus`

```
{
    "properties": {
        "status": {
            "type": "string"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```



## `SELinuxEvent`

```
{
    "properties": {
        "bool": {
            "$ref": "#/definitions/SELinuxBoolChange"
        },
        "enforce": {
            "$ref": "#/definitions/SELinuxEnforceStatus"
        },
        "bool_commit": {
            "$ref": "#/definitions/SELinuxBoolCommit"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```


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
            "type": "string"
        },
        "group": {
            "type": "string"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```




