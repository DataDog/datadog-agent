from test_builder import TestCase
from test_cases.xplat.helpers import confDir


class TestEventLog(TestCase):
    name = "[Windows Event] Agent collect windows event as logs"

    def build(self, config):
        self.append("# Setup")
        self.append(confDir(config))

        self.append(
            """
```
logs:
  - type: windows_event
    channel_path: Testing123
    source: windows.events
    service: myservice
    sourcecategory: windowsevent
```

- Start the agent
- generate some logs:

https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.management/write-eventlog
```
PS C:\\> New-EventLog -LogName Testing123 -Source MyApp
PS C:\\> Write-EventLog -LogName "Testing123" -Source "MyApp" -EventID 3001 -EntryType Information -Message "This is a test event!" -Category 1 -RawData 10,20
```

# Test

- check that the emitted logs show up in app. Only the `Testing123` should appear.
"""
        )
