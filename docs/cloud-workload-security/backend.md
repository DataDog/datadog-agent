# Backend event Documentation

INTRO MSG:
```
BACKEND_EVENT_SCHEMA = {
    "properties": {
        "evt": {
            "$ref": "#/definitions/EventContextSerializer"
        },
        "file": {
            "$ref": "#/definitions/FileEventSerializer"
        },
        "selinux": {
            "$ref": "#/definitions/SELinuxEventSerializer"
        },
        "usr": {
            "$ref": "#/definitions/UserContextSerializer"
        },
        "process": {
            "$ref": "#/definitions/ProcessContextSerializer"
        },
        "container": {
            "$ref": "#/definitions/ContainerContextSerializer"
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
| `evt` | $ref | Please see #/definitions/EventContextSerializer |
| `file` | $ref | Please see #/definitions/FileEventSerializer |
| `selinux` | $ref | Please see #/definitions/SELinuxEventSerializer |
| `usr` | $ref | Please see #/definitions/UserContextSerializer |
| `process` | $ref | Please see #/definitions/ProcessContextSerializer |
| `container` | $ref | Please see #/definitions/ContainerContextSerializer |
| `date` | string |  |



## `ContainerContextSerializer`

INTRO_MSG:
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


## `EventContextSerializer`

INTRO_MSG:
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


## `FileEventSerializer`

INTRO_MSG:
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
            "$ref": "#/definitions/FileSerializer"
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


## `FileSerializer`

INTRO_MSG:
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


## `ProcessCacheEntrySerializer`

INTRO_MSG:
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
            "$ref": "#/definitions/ProcessCredentialsSerializer"
        },
        "executable": {
            "$ref": "#/definitions/FileSerializer"
        },
        "container": {
            "$ref": "#/definitions/ContainerContextSerializer"
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


## `ProcessContextSerializer`

INTRO_MSG:
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
            "$ref": "#/definitions/ProcessCredentialsSerializer"
        },
        "executable": {
            "$ref": "#/definitions/FileSerializer"
        },
        "container": {
            "$ref": "#/definitions/ContainerContextSerializer"
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
            "$ref": "#/definitions/ProcessCacheEntrySerializer"
        },
        "ancestors": {
            "items": {
                "$ref": "#/definitions/ProcessCacheEntrySerializer"
            },
            "type": "array"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```


## `ProcessCredentialsSerializer`

INTRO_MSG:
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


## `SELinuxEventSerializer`

INTRO_MSG:
```
{
    "properties": {
        "bool": {
            "$ref": "#/definitions/selinuxBoolChangeSerializer"
        },
        "enforce": {
            "$ref": "#/definitions/selinuxEnforceStatusSerializer"
        },
        "bool_commit": {
            "$ref": "#/definitions/selinuxBoolCommitSerializer"
        }
    },
    "additionalProperties": false,
    "type": "object"
}
```


## `UserContextSerializer`

INTRO_MSG:
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


## `selinuxBoolChangeSerializer`

INTRO_MSG:
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


## `selinuxBoolCommitSerializer`

INTRO_MSG:
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


## `selinuxEnforceStatusSerializer`

INTRO_MSG:
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


