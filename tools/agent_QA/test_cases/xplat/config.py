from test_builder import Platform, TestCase
from test_cases.xplat.helpers import confDir


class EndpointTests(TestCase):
    name = "[Endpoints] Test endpoint configs"

    def build(self, config):
        self.append("# Setup")
        self.append(confDir(config))
        path = "/var/log/hello-world.log" if config.platform != Platform.windows else "C:\\tmp\\hello-world.log"
        self.append(
            f"""
```
logs:
- type: file
    path: {path}
    service: test-file-tailing
    source: hello-world
```
"""
        )

        self.append(
            """
in your `datadog.yaml`:

```
logs_config:
    logs_dd_url: agent-intake.logs.datadoghq.com:10514
    logs_no_ssl: true
    use_tcp: true
```

- Start the agent
- Generate some logs ``docker run -it bfloerschddog/flog -l > hello-world.log`

# Test
- validate logs are flowing to the intake

*TIP*: Open Live tail and filter by your host. After each test, refresh the page to clear the live tail. You can leave the log producer running between tests

## Repeat the above steps with the following configs\n in `datadog.yaml`

```
logs_config:
    logs_dd_url: agent-intake.logs.datadoghq.com:10516
    logs_no_ssl: false
    use_tcp: true
```

```
logs_config:
    dd_url_443: agent-intake.logs.datadoghq.com
    use_port_443: true
    use_tcp: true
```

```
logs_config:
    api_key: <A DIFFERENT API KEY>
```
"""
        )


class DualShipping(TestCase):
    name = "[Dual Shipping] Test that dual shipping works"

    def build(self, config):  # noqa: U100
        self.append(
            """

### Local QA

1. start 2 mock intakes:
```
docker run --rm -it --name mockhttp1 -p 9998:9998 --rm datadog/agent-dev:bench-logs-intake-dev -b :9998 -d
docker run --rm -it --name mockhttp2 -p 9999:9999 --rm datadog/agent-dev:bench-logs-intake-dev -b :9999 -d
```
the `-d` flag puts them in debug mode so they will stream logs to stdout

2. start an agent:

```
docker run -d --name dd-agent \
    --net=host \
    -e DD_API_KEY="<API_KEY>"\
    -e DD_LOGS_ENABLED=true \
    -e DD_LOGS_CONFIG_LOGS_NO_SSL=true \
    -e DD_LOGS_CONFIG_USE_HTTP=true \
    -e DD_LOGS_CONFIG_LOGS_DD_URL="localhost:9998" \
    -e DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS="[{\"api_key\": \"<API_KEY>\", \"Host\": \"localhost\", \"Port\": 9999, \"is_reliable\": true}]" \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v /proc/:/host/proc/:ro \
    -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
    -v /var/lib/docker/containers:/var/lib/docker/containers:ro \
   <RC_IMAGE>
```

3. get some logs flowing through the agent. Warning if you use `DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL` you will create a feedback loop from the mock intake containers. I'd recommend spawning a dedicated logging container.

4. Watch the logs flow in both intakes.
5. Kill one of the intakes
 - make sure logs are still flowing to the other one

6. Kill the second intake (both are now dead)
 - check `agent status` to see that # of bytes read is not increasing (pipeline should be blocked)

7. Restart one of the intakes - logs should start flowing
8. restart the other intake - logs should start flowing.

### App QA

Now instead of using mock intakes - use two real Datadog intakes. You will need 2 API keys each from different orgs.

```
docker run -d --name dd-agent \
    --net=host \
    -e DD_API_KEY="<API_KEY>"\
    -e DD_LOGS_ENABLED=true \
    -e DD_LOGS_CONFIG_USE_HTTP=true \
    -e DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS="[{\"api_key\": \"<ANOTHER_API_KEY>\", \"Host\": \"agent-intake.logs.datadoghq.com\", \"Port\": 10516, \"is_reliable\": true}]" \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v /proc/:/host/proc/:ro \
    -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
    -v /var/lib/docker/containers:/var/lib/docker/containers:ro \
   <RC_IMAGE>
 ```

Stream some logs and watch the livetail in both orgs for the logs.
"""
        )
