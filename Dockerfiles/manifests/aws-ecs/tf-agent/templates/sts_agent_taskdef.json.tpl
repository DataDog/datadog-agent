{
  "containerDefinitions": ${sts_agent_taskdef_containers},
  "volumes": [
    {
      "host": {
        "sourcePath": "/var/run/docker.sock"
      },
      "name": "docker_sock"
    },
    {
      "host": {
        "sourcePath": "/proc/"
      },
      "name": "proc"
    },
    {
      "host": {
        "sourcePath": "/cgroup/"
      },
      "name": "cgroup"
    },
    {
      "host": {
        "sourcePath": "/etc/passwd"
      },
      "name": "passwd"
    },
    {
      "host": {
        "sourcePath": "/sys/kernel/debug"
      },
      "name": "kerneldebug"
    }
   ],
  "family": "${sts_agent_task_family}"
}
