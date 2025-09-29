# Stateful Log Encoding Design

## Overview

This document describes the various key points regarding the stateful handling at Agent.

## Types of State

States can be patterns or a string dictionary. Patterns are self-explanatory. The string dictionary is used to store repeated strings from dynamic log values, as well as key/value strings from tags. For example:

```
Log: "Starting scheduled run to download latest context for INTAKE_AUTH_CONTEXT"
pattern: "Starting scheduled run to download latest context for <$1>"
string dictionary: "INTAKE_AUTH_CONTEXT"
```

We can defer the decision on whether to use separate "namespaces" in the string dictionary to isolate storage for dynamic values, tag keys, and tag values.

## Operations on the State

Each entry in the state can change in the following way:

- **Insert**: New state entries are created; applies to both patterns and the string dictionary.
- **Delete**: To stay within state size constraints, lower-value entries (in terms of bandwidth savings) may be evicted. Applies to both patterns and the string dictionary.
- **Update**: Applies only to patterns. Existing patterns can evolve as new logs arrive. For example:

```
t1: "User A signed on from 192.168.1.1"
t2: "User A signed on from 192.168.1.2" 
    <-- here the pattern is "User A signed on from <$1>"
t3: "User B signed on from 192.168.1.3"
    <-- here the pattern becomes "User <$1> signed on from <$2>"
```

## Logs Processing & State Modification

The processing of the logs is roughly as follows:

1. All existing log processing steps are performed as usual, up to the point where the log is ready to be encoded and sent.
2. The log then goes through a pattern extraction step to determine whether:
   - The log should be sent as-is (raw string), or
   - The log can be decomposed into pattern and dynamic values
3. The log's tags and any dynamic string values are passed through the string dictionary, replacing them with dictionary indices where applicable
4. The steps in 2 and 3 can trigger state operations, such as delete/add/modify patterns and delete/add string dictionary. For example:

```
t1: "User A signed on from 192.168.1.1"
    <-- logs is sent as-is
t2: "User A signed on from 192.168.1.2" 
    <-- new pattern: "User A signed on from <$1>"
    <-- if state is full, an existing pattern is evicted
t3: "User B signed on from 192.168.1.3"
    <-- pattern modification: "User <$1> signed on from <$2>"
```

## Encoding & Batching

If state modifications occurred (example above), then the modification must be encoded in order and before the log – since the log is encoded based on the updated state. We continue to follow the agent's existing batching rules (batch size, count, and timeout).

Using the example from previous section, the content of the batch might look like this (each line presents either a state change or a log):

```
[raw log]  User A signed on from 192.168.1.1
[pattern delete] No.2
[pattern create] No.10 "User A signed on from <$1>"
[pattern log] pattern:No.10, dynamic value: 192.168.1.2
[pattern update] No.10 "User <$1> signed on from <$2>"
[pattern log] pattern:No.10, dynamic value: B, 192.168.1.3
```

For detailed wire format of the encoding, see the section on the protobuf definition.

## Failures & Retransmission

The server side (Intake) depends on strictly ordered delivery of state changes and logs to maintain synchronization, decode incoming data, and reconstruct logs correctly. In the normal (non-failure) case, we rely on the gRPC stream to provide both ordering and delivery guarantees.

However, there are situations where retransmission — of state or logs — must be handled explicitly at the application layer. These include:

- **Normal stream termination**: The Agent closes the stream after reaching its configured stream lifetime
- **Abnormal stream termination**: The stream is terminated or reset by Intake or intermediate proxies. This can happen for various reasons — such as server restarts, load rebalancing, or non-successful responses (e.g., HTTP 503).

In either case, the following recovery steps are executed:

1. Establish a new gRPC stream
2. Transmit the current state: pattern + string dictionary (as creation)
3. Re-transmit any in-flight logs

To identify the in-flight logs, every batch's request and response contains a batch-id. If the stream fails before a batch is acknowledged, that batch-id is considered unconfirmed and must be retried.

**IMPORTANT**: retransmission does not mean replaying the original encoded request as-is. Consider this scenario:

```
t1: state contains pattern P1 and P2
t2: log1 encoded by P1
t3: P1 evicted, state changes to P2 and P3
    log2 encoded by P3
t4: stream is cut
t5: new stream is established
t6: retransmit state: [P2 and P3]
t7: retransmit log1 and log2
```

If we simply replay the original encoded version of log1 (from t2), decoding will fail — because P1 no longer exists in the re-initialized state (t6). To handle this correctly, we have two options:

1. Implement a more complex state update protocol to handle the state change attributed to the in-flight/unack-ed batches, or
2. Require that every retransmitted log be re-encoded using the current state before being sent

We will use the latter for the moment, for simplicity.

Transport layer errors close the gRPC stream. However, there may be failure scenarios where we receive an error response (in the BatchStatus message from server), but the gRPC stream itself remains open and functioning; for example:

```
client: send batch1={pattern1, log1, log2, log3})
server: process batch and respond: batchStatus{id=1, status=ok}
client: send batch2={...}
server: overloaded - immediately drop batch2 and respond: bachStatus{id=2, status=unavailable, details="server overloaded - retry again with backoff"}
    <x seconds pass>
    client: send(batch2)
    server: process batch and respond: batchStatus{id=2, status=ok}
```

In such cases, it is safe to resend the previously encoded request as-is, without going through the potentially expensive re-patterning and re-encoding steps. We plan to explore this as a possible optimization in future iterations.

## State Life Cycle

**On the Agent side**: The state lifecycle is tied to the lifetime of the Agent process. State is maintained in memory for the duration of the process, with no persistence across restarts. When the Agent restarts, it begins with an empty state and rebuilds it through ongoing log processing. However, state does persist across gRPC streams — meaning that each time a new stream is created, the Agent must retransmit its current state (as described above).

**On the Intake side**: State is tied to the lifetime of the gRPC stream. If the stream is terminated for any reason, the associated state is cleared. Intake expects that a new stream will be initialized with a full state transmission from the Agent.

## State Entry Addition/Eviction

TODO [This can be added later]

## State/Stream Parallelism

There are several ways to architect this, but the simplest approach is likely the following:

1. Maintain separate state per stream, which simplifies error handling — if a stream breaks, it's clear which state needs to be retransmitted
2. Leverage existing concurrency mechanisms and extend them to streams. The Agent already manages concurrency through multiple pipelines (with the number of pipelines based on CPU cores). Each log source (e.g., a log file) is assigned to a pipeline in a round-robin fashion. This model already provides parallelism and log source–level locality, which is beneficial for pattern extraction. We can build state management at the pipeline level and attach a dedicated gRPC stream for output from each pipeline.

**Note**: The proposed approach will likely result in unevenly sized gRPC streams. For example, consider a main container with heavy logging activity versus a small sidecar container that only logs during startup and shutdown — if each container's logs are routed to a different gRPC stream, the throughput between streams can differ significantly. We'll defer this concern to Stage 2 of the PoC, where we can explore potential strategies to address this at the proxy level.

## Error Handling

### Errors in Bidirectional gRPC Streaming

Bidirectional gRPC streams support fully async client -> server and server <- client communication, so are typically represented by two threads:

**Client:**
```go
// Start a goroutine to receive messages from the server
go func() {
    for {
        msgFromServer, err := stream.Recv()
        if err == io.EOF {
            // Stream closed.
            close(waitc)
            return
        }
        if err != nil {
            log.Fatalf("Failed to receive : %v", err)
        }
        log.Printf("Got message: %s", in.GetContent())
    }
}()

// In a goroutine, forward batches of logs to Intake
go func() {
    for logBatch := range batcherChan {
        encodedBatch := encode(logBatch)
        err := stream.Send(encodedBatch)
        if err != nil {
            log.Fatalf("Failed to send batch: %v", err)
        }
    }
}()
```

**Server:**
```go
func (s *server) LogsStream(stream pb.StatefulLogsService_LogsStream) error {
    log.Println("Stateful LogsStream opened")
    for {
        // Receive a message from the client
        req, err := stream.Recv()
        if err == io.EOF {
            // The client has closed the stream
            return nil
        }
        if err != nil {
            log.Printf("Error while reading client stream: %v", err)
            return err
        }

        err := submitBatch(req)
        if err != nil {
            return err
        }

        // Send a message back to the client stream
        response := &pb.BatchStatus{
            Id: ...,
        }
        if err := stream.Send(response); err != nil {
            log.Printf("Error while sending data to client: %v", err)
            return err
        }
    }
}
```

### Important Notes

- Transport level errors always close the stream
- Unlike unary RPC, stream.Send(<msg>) cannot be retried on e.g. "UNAVAILABLE" errors since gRPC is unable to auto-reconnect to the same backend.
- Likewise, when a server sends back an error (by returning an error in the stream handler) the stream gets closed
- The client sees the error+status on stream.Recv(), and future stream.Send(_) calls receive io.EOF
- If we want to communicate non-success but keep the stream open we must send it via an actual message. For gRPC Internal Intake, we currently send Status.UNAVAILABLE to represent backpressure. If we want to backpressure without closing streams we must express that via the BatchStatus response instead
- gRPC stream messages are delivered in order (i.e. message N is always received before message N+1 when the server calls stream.Recv()).

### Status Code Mappings

As noted above the protocol supports two ways of returning application layer errors:

1. Returning an error in server stream handler - this closes the stream from the server.
2. Sending an error in the BatchStatus response - this allows the client to decide to close the stream or keep it open

Our general thought process on error handling will be:

**Non-retryable errors (auth, malformed request, etc):**
- Return the status in BatchStatus
- Client should not re-establish stream
- Why not reopen the stream? if an agent is repeatedly sending e.g. requests without an API key there is no need to repeatedly open new connections for the same result.

**Retryable errors (backpressure, other internal errors, state size too big):**
- Return the status in BatchStatus
- Once Intake returns a retryable error on a message it should return errors on future batches to avoid missing gaps in the data
- Why not just close the stream on server? Like above If the client is misbehaving and consistently sending invalid messages it is presumably cheaper to continually send back errors than repeatedly force a bad client to re-establish a stream.
- Client should re-establish stream and start sending again
- Eventually we may add to the protocol to allow recovery from retryable errors within a stream
- E.g. resend original failed batch and all batches after.

### HTTP Response Code to BatchStatus Mapping

- **202 Accepted** => BatchStatus[status=OK]
  - Request was written successfully downstream (or to journal to be written later)

- **400 Bad Request** => BatchStatus[status=INVALID_ARGUMENT] (non-retryable)
  - Occurs on HTTP Intake when e.g. request aggregation fails or on invalid content type for a track. Any invalid StatefulBatch sent from the client should yield this result.

- **401 Unauthorized** => BatchStatus[status=UNAUTHENTICATED]** (non-retryable)
  - Occurs when a request does not attach an API key.

- **403 Forbidden** => BatchStatus[status=PERMISSION_DENIED]** (non-retryable)
  - Occurs when client presents an invalid/denylisted API key (or potentially if the track is not configured correctly or PCI compliance) issues.

**at some point authentication at edge will turn this into a stream level error. How the stateful protocol interacts with auth@edge can be discussed further in a different doc.

- **500 Internal Server Error** => BatchStatus[status=INTERNAL] (retryable)
  - 500 is a catchall for any unhandled exception that occurs at Intake. This should be considered a bug at intake but can be retried.

- **BatchStatus[status=STATE_TOO_BIG]** (non-retryable, at least with the same request)
  - STATE_TOO_BIG (or potentially the standard code RESOURCE_EXHAUSTED) may also be sent if the state grows too big on the server. Future iterations of the API may want to handle this more sophisticatedly (e.g. server warns about state size followed by client removing patterns).

- **Backpressure (HTTP 429 or 503)** => BatchStatus[status=OVERLOADED] (retryable)
  - Backpressure refers to the Intake load shedding mechanism where Intake rejects requests as early as possible when under load. Two types of rejections exist - targeted when Intake detects a single peaking org is the cause of overload (429), and general when Intake rejects requests from overall server load (503). See Page: Targeted backpressure at Intake for more details.
  - When overloaded the stream handler will drop a received request and return the overloaded status ASAP to avoid further processing the message. The server will also close the stream (clearing relevant state) to shed load.

### HTTP Response Code to gRPC Stream Errors (non-retryable)

- **408 Timeout** => io.EOF, Status.DEADLINE_EXCEEDED, Status.UNAVAILABLE
  - Public Intake responds with a timeout when a single request takes too long (e.g. the full HTTP2 request takes too long to come in or the response is too slow to write back to client). The gRPC library takes care of this specific case (client would see DEADLINE_EXCEEDED on send).

Other related cases here:
- The Intake handler may close a stream with no new requests after X seconds. Closing with no error will show the client io.EOF.
- After X minutes (depending on LB parameters) Intake or the LB may close a longrunning stream/request. Again we can close the stream gracefully here using io.EOF/without an explicit failed gRPC status
- gRPC has settings around max connection age (and a grace period for connections with ongoing RPC calls) that will close existing streams with UNAVAILABLE when reached.
- Page: Best practices for gRPC long lived streams | Rebalancing streams without errors discusses how can potentially handle this case gracefully with io.EOF

- **413 Request too large** => Status.RESOURCE_EXHAUSTED/UNKNOWN
  - Relatively straightforward, but worth noting that this is handled by the gRPC library not our application logic. Additionally, when using transport compression the Java gRPC library returns an UNKNOWN error on large requests instead of RESOURCE_EXHAUSTED https://github.com/grpc/grpc-java/issues/11246.
