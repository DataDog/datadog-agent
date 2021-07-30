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
            "type": "string"
        },
        "name": {
            "type": "string"
        },
        "path_resolution_error": {
            "type": "string"
        },
        "inode": {
            "type": "integer"
        },
        "mode": {
            "type": "integer"
        },
        "in_upper_layer": {
            "type": "boolean"
        },
        "mount_id": {
            "type": "integer"
        },
        "filesystem": {
            "type": "string"
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
        "attribute_name": {
            "type": "string"
        },
        "attribute_namespace": {
            "type": "string"
        },
        "flags": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "access_time": {
            "type": "string",
            "format": "date-time"
        },
        "modification_time": {
            "type": "string",
            "format": "date-time"
        },
        "change_time": {
            "type": "string",
            "format": "date-time"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```


## `FileEvent`

```
{
    "required": [
        "uid",
        "gid"
    ],
    "properties": {
        "path": {
            "type": "string"
        },
        "name": {
            "type": "string"
        },
        "path_resolution_error": {
            "type": "string"
        },
        "inode": {
            "type": "integer"
        },
        "mode": {
            "type": "integer"
        },
        "in_upper_layer": {
            "type": "boolean"
        },
        "mount_id": {
            "type": "integer"
        },
        "filesystem": {
            "type": "string"
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
        "attribute_name": {
            "type": "string"
        },
        "attribute_namespace": {
            "type": "string"
        },
        "flags": {
            "items": {
                "type": "string"
            },
            "type": "array"
        },
        "access_time": {
            "type": "string",
            "format": "date-time"
        },
        "modification_time": {
            "type": "string",
            "format": "date-time"
        },
        "change_time": {
            "type": "string",
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



