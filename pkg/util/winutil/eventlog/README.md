# Windows Event Log package

These package(s) interact with the [Windows Event Log API](https://learn.microsoft.com/en-us/windows/win32/wes/windows-event-log)

* [evtsubscribe](subscription) - Implements a [Pull Subscription](https://learn.microsoft.com/en-us/windows/win32/wes/subscribing-to-events#pull-subscriptions)
* [evtbookmark](bookmark) - [Bookmarking Events](https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events)

See the [example usage](example_test.go).

## APIs

The [evtapi.API](api) interface includes functions from both the legacy [Event Logging API](https://learn.microsoft.com/en-us/windows/win32/eventlog/event-logging) as well as the newer [Windows Event Log API](https://learn.microsoft.com/en-us/windows/win32/wes/windows-event-log).


## Testing

The [eventlog_test.APITester](test) interface provides helpers for writing tests that need to install/remove event logs/channels, sources, and generate events.

Tests can be run using either the [Windows API](api/windows) or the [Fake API](api/fake), selected by the `-evtapi` argument to `go test`. By default the tests are run only with the [Fake API](api/fake) to avoid inadvertently modifying the host event logs.

### Unit tests

Simply run `go test ./...` to run the tests with the [Fake API](api/fake).

### Integration tests

The tests can be run with the [Windows API](api/windows), which will install/remove event logs on the system and fill them with events.

The integration tests can be run directly `go test ./... -evtapi Windows`, or through the invoke task `inv -e integration-tests`.
