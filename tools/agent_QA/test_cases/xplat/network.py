from test_builder import TestCase
from test_cases.xplat.helpers import confDir


# TODO - add windows specific commands
class TailTCPUDP(TestCase):
    name = "[TCP/UDP] Agent can collect logs from TCP or UDP"

    def build(self, config):
        self.append("# Setup")
        self.append(confDir(config))
        self.append(
            """
```
logs:
  - type: tcp
    port: 10514
    service: tcp-listener
    source: tcp
  - type: udp
    port: 10515
    service: udp-listener
    source: udp
```

- Start the agent

# Test
```
echo '154.87.78.229 - - [02/Feb/2021:10:54:52 +0100] "PATCH /facilitate HTTP/1.0" 504 23565' > /dev/udp/localhost/10515
```
- validate UDP logs show in app

```
echo '95.154.19.214 - ratke2841 [02/Feb/2021:10:57:48 +0100] "GET /real-time/disintermediate HTTP/1.0" 504 5641'  > /dev/tcp/localhost/10514
```
- validate TCP logs show in app
"""
        )
