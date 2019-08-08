[
    {
      "name": "stackstate-agent",
      "image": "${sts_agent_image}",
      "cpu": 10,
      "memory": 256,
      "essential": true,
      "privileged": true,
      "networkMode": "host",
      "linuxParameters": {
          "capabilities": {
              "add": [
                  "SYS_ADMIN"
              ],
              "drop": []
          }
      },
      "mountPoints": [
        {
          "containerPath": "/var/run/docker.sock",
          "sourceVolume": "docker_sock",
          "readOnly": true
        },
        {
          "containerPath": "/host/sys/fs/cgroup",
          "sourceVolume": "cgroup",
          "readOnly": true
        },
        {
          "containerPath": "/host/proc",
          "sourceVolume": "proc",
          "readOnly": true
        },
        {
          "containerPath": "/etc/passwd",
          "sourceVolume": "passwd",
          "readOnly": true
        },
        {
          "containerPath": "/sys/kernel/debug",
          "sourceVolume": "kerneldebug",
          "readOnly": false
        }
      ],
      "environment": [
        {
          "name": "STS_API_KEY",
          "value": "${STS_API_KEY}"
        },
        {
          "name": "STS_PROCESS_AGENT_ENABLED",
          "value": "${STS_PROCESS_AGENT_ENABLED}"
        },
        {
          "name": "STS_STS_URL",
          "value": "${STS_URL}"
        },
        {
          "name": "STS_PROCESS_AGENT_URL",
          "value": "${STS_URL}"
        },
        {
          "name": "LOG_LEVEL",
          "value": "${LOG_LEVEL}"
        },
        {
          "name": "STS_LOG_LEVEL",
          "value": "${LOG_LEVEL}"
        },
        {
          "name": "STS_NETWORK_TRACING_ENABLED",
          "value": "true"
        },
        {
          "name": "STS_SKIP_SSL_VALIDATION",
          "value": "${STS_SKIP_SSL_VALIDATION}"
        },
        {
          "name": "HOST_PROC",
          "value": "/host/proc"
        },
        {
          "name": "HOST_SYS",
          "value": "/host/sys"
        },
        {
          "name": "STS_PROCESS_BLACKLIST_PATTERNS",
          "value": "^s6-,^docker-,^/usr/sbin"
        }

      ]
    }
]
