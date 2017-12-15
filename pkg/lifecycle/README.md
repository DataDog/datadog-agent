# Lifecycle

## Goals

Identify if the datadog-agent is healthy by creating readiness && liveness features.

Expose a HTTP endpoint to allow external services to reach easily the status of the agent.
If running in a systemd `.service`: notify dbus the readiness of the application.


### HTTP Endpoint

Implemented HTTP responses on GET `/health`:

* **200** `{"health":true}`
* **503** `{"health":false}`
* **500** `{"health":false}`

Healthy response example from the agent:
       
```bash 
curl -fv 127.0.0.1:5000/health 
*   Trying 127.0.0.1...
* TCP_NODELAY set
* Connected to 127.0.0.1 (127.0.0.1) port 5000 (#0)
> GET /health HTTP/1.1
> Host: 127.0.0.1:5000
> User-Agent: curl/7.52.1
> Accept: */*
> 
< HTTP/1.1 200 OK
< Content-Type: application/json
< Date: Sun, 26 Nov 2017 17:01:06 GMT
< Content-Length: 15
< 
* Curl_http_done: called premature == 0
* Connection #0 to host 127.0.0.1 left intact
{"health":true}
```

### Notify Systemd

Online documentation: https://www.freedesktop.org/software/systemd/man/systemd-notify.html

##### TL;DR:

> systemd-notify may be called by daemon scripts to notify the init system about status changes
> Most importantly, it can be used for start-up completion notification.

> Tells the service manager that service startup is finished. This is only used by systemd if the service definition file has Type=notify set.

##### Example of notify service file

```text
[Unit]
After=network.target

[Service]
Type=notify
Environment=DD_API_KEY=******
EnvironmentFile=-/etc/datadog-agent-env
ExecStart=/path/to/datadog-agent start $ARGS
Restart=on-failure
RestartSec=2s

[Install]
WantedBy=multi-user.target
```

##### Behavior 

Start the service:

```bash
systemctl start datadog-agent.service 
```
    
The Active field will have two states:

* activating (start)
* active (running)  
    
The activating is triggered by the `start` command, the active by the sd_notify(3) call in the agent's code.  
    
When active, you can observe systemd PID 1 writing in the logs of `datadog-agent.service` the following entry:

```json
{        
    "PRIORITY" : "6",
    "SYSLOG_FACILITY" : "3",
    "SYSLOG_IDENTIFIER" : "systemd",
    "CODE_FILE" : "../src/core/job.c",
    "CODE_LINE" : "804",
    "CODE_FUNCTION" : "job_log_status_message",
    "RESULT" : "done",
    "_TRANSPORT" : "journal",
    "_PID" : "1",
    "_COMM" : "systemd",
    "_EXE" : "/lib/systemd/systemd",
    "_CMDLINE" : "/sbin/init splash",
    "_SYSTEMD_CGROUP" : "/init.scope",
    "_SYSTEMD_UNIT" : "init.scope",
    "_SYSTEMD_SLICE" : "-.slice",
    "UNIT" : "datadog-agent.service",
    "MESSAGE" : "Started datadog-agent.service."
}
```