from test_builder import Platform, TestCase
from test_cases.xplat.helpers import confDir, filePositionSharedSteps


class TailFile(TestCase):
    name = "[Files] Agent can tail a file"

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
- Start the agent")
- generate some logs `docker run -it bfloerschddog/flog -l > hello-world.log`

# Test
- Validate the logs show up in app with the correct `source` and `service` tags
- Block permission to the file (`chmod 000 hello-world.log`) and check that the Agent status shows that it is inaccessible. 
- Chang the permissions back so it is accessible again. 
- Stop the agent, generate new logs, start the agent and make sure those are sent.
- Rotate the log file (`mv hello-world.log hello-world.log.old && touch hello-world.log`), ensure that logs continue to send after rotation. 
"""
        )


class TailFileMultiLine(TestCase):
    name = "[Files] Agent can tail multi line logs"

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
    source: multiline
    log_processing_rules:
      - type: multi_line
        name: new_log_start_with_date
        pattern: \\d{{4}}\\-(0?[1-9]|1[012])\\-(0?[1-9]|[12][0-9]|3[01])
``` 
"""
        )

        self.append(
            """
- Start the agent
- generate some multi-line logs `docker run -it bfloerschddog/java-excepton-logger` > hello-world.log`

# Test
- Validate that the logs show up in app correctly. Look for the multi-line exception logs and ensure they are combined into a single log line. 
"""
        )


class TailFileUTF16(TestCase):
    name = "[Files] Agent can tail UTF16 files"

    def build(self, config):

        self.append("# Setup")
        self.append(confDir(config))

        path = "/var/log/hello-utf16.log" if config.platform != Platform.windows else "C:\\tmp\\hello-utf16.log"
        self.append(
            f"""
```
logs: 
- type: file
    path: {path}
    service: test-file-tailing
    source: hello-world
    encoding: utf-16-le
``` 
"""
        )

        self.append(
            """
- Start the agent

# Test
- Generate UTF16-le logs `python -c "f = open('hello-utf16.log', 'ab'); t='This is just sample text2\n'.encode('utf-16'); f.write(t); f.close()"`
- check that the logs look correct in app
- delete the log file, change the config to `encoding: utf-16-be`, and restart the agent
- Generate UTF16-be logs `python -c "f = open('hello-utf16.log', 'ab'); t='This is just sample text2\n'.encode('utf-16be'); f.write(t); f.close()"`
- check that the logs look correct in app
"""
        )


class TailFileWildcard(TestCase):
    name = "[Files] Agent can use wildcards to tail a file"

    def build(self, config):

        self.append("# Setup")
        self.append(confDir(config))

        path = "/var/log/*.log" if config.platform != Platform.windows else "C:\\tmp\\*.log"
        self.append(
            f"""
```
logs:
  -type: file
    path: {path}
    service: test-wildcard
    source: wildcard
``` 
"""
        )

        self.append(" - Start the agent")
        self.append(
            """ - generate some logs in multiple files:
- `docker run -it bfloerschddog/flog -l > 1.log`
- `docker run -it bfloerschddog/flog -l > 2.log`
- `docker run -it bfloerschddog/flog -l > 3.log`

# Test
- the tag `filename` tag is set on the log metadata
- the tag directory name tag is set on the log metadata
- Change the `logs_config.open_files_limit` to 1 in `datadog.yaml`, restart the agent and make sure the agent is only tailing 1 file
"""
        )


class TailFileStartPosition(TestCase):
    name = "[Files] `start_position` defines where to tail from"

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
    start_position: beginning
``` 
"""
        )

        self.append(
            """# Test
1. start the agent
2. generate some logs like `docker run -it bfloerschddog/flog -l > hello-world.log`
3. check the logs show up in app
4. stop the agent. 
"""
        )
        self.append(filePositionSharedSteps())
