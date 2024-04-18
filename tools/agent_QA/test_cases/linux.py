from test_builder import TestCase
from test_cases.xplat.helpers import confDir, filePositionSharedSteps


class TailJounald(TestCase):
    name = "[Journald] Agent collect logs from journald"

    def build(self, config):
        self.append("# Setup")
        self.append(confDir(config))

        self.append(
            """
```
logs:
  -type: journald
```
"""
        )

        self.append(" - Start the agent")
        self.append(" - generate some logs `echo 'hello world' | systemd-cat`")

        self.append(
            """# Test
- check that the `hello world` log shows up in app

update the config:

```
logs:
  - type: journald
    path: /var/log/journal/
    include_units:
      - docker.service
      - sshd.service
```

- confirm in app that you are able to filter by specific units
"""
        )


# TODO: improve test clarity for per unit filtering


class TailJournaldStartPosition(TestCase):
    name = "[Files] `start_position` defines where to tail from"

    def build(self, config):
        self.append("# Setup")
        self.append(confDir(config))

        self.append(
            """
```
logs:
    - type: journald
      start_position: beginning
```

# Test

1. start the agent
2. generate some logs like `echo 'test message' | systemd-cat`
3. check the logs show up in app
4. stop the agent.
"""
        )
        self.append(filePositionSharedSteps())


class SNMPTraps(TestCase):
    name = "[SNMP traps] Check that traps are working"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup
Agent support snmp traps listening:

```
api_key: "******"

logs_enabled: true
# logs_config:
#   # Override if you'd like to send to eg staging
#   logs_dd_url: <STAGING_LOGS_INTAKE_URL>

snmp_traps_enabled: true
snmp_traps_config:
  port: 1620
  community_strings:
    - public
```

And to send a trap from net-snmp:

```
snmptrap -v 2c -c public localhost:1620 '' NET-SNMP-EXAMPLES-MIB::netSnmpExampleHeartbeatNotification netSnmpExampleHeartbeatRate i 123456

```

# Test

- check livetail/in app that the logs show up

"""
        )
