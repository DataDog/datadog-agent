# GPU event viewer

This is a debug utility that can be used to view GPU events recorded using the `collect-events` debug endpoint.

## Usage

Record the GPU events from a system-probe process:

``bash
curl --unix-socket $DD_SYSPROBE_SOCKET http://unix/gpu/debug/collect-events?count=100 > events
```

Build and use the event viewer:

```bash
dda inv system-probe.build-gpu-event-viewer
pkg/gpu/testutil/event-viewer/event-viewer path-to-events-file
```
