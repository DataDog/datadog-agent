# Flight Recorder Connection Lifecycle Design

## Problem Statement

The Go agent and Rust sidecar communicate over a Unix domain socket using
a length-prefixed FlatBuffers protocol. The connection lifecycle must handle:

1. Sidecar not ready yet (agent starts before sidecar)
2. Sidecar restarts (socket recreated)
3. Transient write failures (backpressure, timeout)
4. Permanent write failures (broken pipe, partial write corrupting framing)
5. Agent reconnection without losing hook subscriptions or batcher state

Current design tears down everything (hooks, batcher, transport) on any
connection issue, which is too expensive and causes data loss during the
restart cycle.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│ Go Agent (sinkImpl)                                     │
│                                                         │
│  ┌──────────┐     ┌──────────┐     ┌──────────────────┐│
│  │ Hook      │────>│ Batcher  │────>│ Transport        ││
│  │ callbacks │     │ (rings + │     │ (socket + send)  ││
│  │           │     │  encode) │     │                  ││
│  └──────────┘     └──────────┘     └──────────────────┘│
│   Created once     Created once     Owns connection,   │
│   per activate()   per activate()   handles reconnect  │
│                                     internally          │
└─────────────────────────────────────────────────────────┘
        │                                    │
        │ Unix domain socket (SOCK_STREAM)   │
        │ Length-prefixed FlatBuffers frames  │
        │                                    │
┌───────▼────────────────────────────────────▼────────────┐
│ Rust Sidecar                                            │
│                                                         │
│  ┌──────────────┐  rtrb   ┌───────────────────────────┐│
│  │ Async handler │───────>│ Writer thread (per signal) ││
│  │ (per conn)    │        │ decode → accumulate →flush ││
│  └──────────────┘         └───────────────────────────┘│
│   Spawned per accept()    Persistent across connections │
└─────────────────────────────────────────────────────────┘
```

## Connection States (Go Agent Side)

```
                    ┌─────────┐
         start ───>│ PROBING  │ discovery loop: DialTimeout every N seconds
                    └────┬────┘
                         │ socket found
                         ▼
                    ┌──────────┐
                    │CONNECTED │ Send() works, flush loop active
                    └────┬─────┘
                         │
              ┌──────────┼──────────────┐
              │          │              │
              ▼          ▼              ▼
         zero-byte   partial       non-timeout
         timeout     write         error (EPIPE,
         (EAGAIN)    (n>0,n<len)   ECONNRESET)
              │          │              │
              │          ▼              ▼
              │     ┌──────────┐  ┌──────────┐
              │     │RECONNECT │  │RECONNECT │
              │     └────┬─────┘  └────┬─────┘
              │          │              │
              │          ▼              ▼
              │     DialTimeout    DialTimeout
              │     to same path   to same path
              │          │              │
              │     ┌────┴──────────────┘
              │     │
              │     ├── success ──> CONNECTED (new fd, same batcher)
              │     │
              │     └── fail ────> DISCONNECTED
              │                         │
              │                    full teardown
              │                    (hooks, batcher)
              │                    back to PROBING
              │
              └── drop frame, stay CONNECTED
```

## Error Classification

### Drop frame, keep connection (no-op)

- **Zero-byte timeout**: `WriteTo` returns `(0, timeout_error)`. The kernel
  socket buffer was full, no bytes entered the stream. The framing is clean.
  Drop the current frame, continue with the next flush cycle.

**How to detect**: `err != nil && n == 0 && err.Timeout()`

### Reconnect silently (replace socket fd)

- **Partial write**: `WriteTo` returns `(n, error)` where `0 < n < frameLen`.
  Some bytes are in the kernel buffer, the framing is corrupt. The old
  connection must be closed and a new one opened.

- **Broken pipe / connection reset**: `WriteTo` returns `(0, EPIPE)` or
  `(0, ECONNRESET)`. The sidecar closed its end. The socket fd is dead.

**How to detect**: `err != nil && (n > 0 || !err.Timeout())`

**Reconnect behavior**:
1. Close old connection (fd)
2. `DialTimeout("unix", socketPath, 2s)`
3. If success: store new conn, return error for current frame (lost)
4. If fail: set conn = nil, next Send() returns errNotConnected

**What stays alive**: batcher (rings, flush loop, builder pool), hook
subscriptions, seenContexts bloom filter, counters.

**What the sidecar sees**: old connection handler gets EOF, exits cleanly.
New connection handler spawned by accept(). Writer threads are unaffected
(they're per-signal-type, not per-connection).

### Full teardown (fatal)

- **Reconnect fails**: the socket path doesn't exist (sidecar is gone).
  Signal the discovery loop to restart from scratch.

- **Context cancelled**: agent shutdown.

**What happens**: unsubscribe hooks, stop batcher (drain + close), close
transport. Discovery loop starts probing again with backoff.

## Sidecar Side Connection Handling

### Current behavior

Each `accept()` spawns a new async task:

```rust
tokio::spawn(async move {
    let mut reader = BufReader::new(stream);
    loop {
        let buf = read_frame(&mut reader).await?;
        match payload_type {
            MetricBatch => metrics_handle.send_frame(buf),
            LogBatch => logs_handle.send_frame(buf),
            TraceStatsBatch => traces_handle.send_frame(buf),
        }
    }
});
```

### What happens on agent reconnect

1. Old connection: `read_frame()` returns `Ok(None)` (EOF) or `Err`. Handler
   task exits. `active_connections` counter decrements.
2. New connection: `accept()` fires, new handler task spawned.
   `active_connections` counter increments.
3. Writer threads: unaffected. They process frames from the rtrb ring
   regardless of which connection produced them.
4. Context store: bloom filter persists. Duplicate context definitions
   from the reconnected agent are deduplicated. No data loss.

### Invariant

The sidecar does NOT reset any state on new connections. The bloom filter,
Parquet writers, and context store all persist. This is safe because
context keys are deterministic hashes.

## Wire Protocol

```
[4 bytes LE: payload length][payload bytes (FlatBuffers SignalEnvelope)]
```

### Partial write problem

`net.Buffers.WriteTo` uses `writev()` which writes both the 4-byte prefix
and the payload in one syscall when possible. But the kernel may split the
write (returns partial count) if:

- Socket buffer is nearly full (only N bytes fit)
- Signal interrupts the syscall
- Write deadline fires mid-write

After a partial write, the sidecar's frame reader is permanently
desynchronized — it reads wrong bytes as the length prefix, producing
garbage frames ("Range out of bounds", "unaligned" FlatBuffers errors).

**Recovery**: the only option is to close the connection and open a new
one. There is no way to resynchronize a length-prefixed stream after a
partial write.

### Prevention strategies (future)

1. **Sentinel-based framing**: use a magic byte sequence instead of length
   prefix. Allows resynchronization after corruption. More complex to
   implement.

2. **SOCK_SEQPACKET**: preserves message boundaries at the kernel level.
   Each `send()` is atomic — either the full message is delivered or
   nothing. Max message size = socket buffer size (~208 KB).
   Trade-off: limits frame size to socket buffer, requires both sides
   to use seqpacket.

3. **Frame size cap**: keep frames under the socket buffer size (~128 KB).
   If a single `write()` fits in the buffer, it completes atomically.
   The chunked flush (maxChunkSize = 2000) already limits frame size,
   but context defs with many tags can still exceed this.

## Open Questions

1. Should `reconnect()` be synchronous (block the flush goroutine for up
   to 2s) or async (set conn=nil, let a background goroutine reconnect)?

2. Should we add a reconnect backoff? If the sidecar is restarting, rapid
   reconnect attempts waste CPU.

3. Should the transport expose a `IsConnected()` method so the batcher can
   skip encoding when disconnected?

4. Should we adopt SOCK_SEQPACKET to eliminate partial write risk entirely?
   This would require capping all frames to ~128 KB and changing both the
   Go and Rust socket setup.
