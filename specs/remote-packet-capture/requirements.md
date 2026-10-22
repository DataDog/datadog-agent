# Remote packet capture

## User story

As a network operator, I need to obtain a bounded packet-header capture from a host
without shell access, so that I can investigate network behavior without collecting
application payloads.

## Requirements

### REQ-PCAP-001: Request targeted evidence without shell access

WHEN an operator invokes the remote capture action with a valid duration, THE SYSTEM
SHALL capture traffic from the requested host interface and apply the supplied traffic
filter.

WHEN no interface is specified, THE SYSTEM SHALL capture across the host’s interfaces.

WHEN no traffic filter is specified, THE SYSTEM SHALL admit all traffic visible to the
capture interface.

IF the filter is invalid or cannot be applied to the selected interface, THEN THE SYSTEM
SHALL fail the capture request.

**Rationale:** Operators need to collect evidence around the affected traffic without
logging into the machine or silently receiving a broader capture than requested.

### REQ-PCAP-002: Enable capture deliberately

WHILE packet capture is not explicitly enabled, THE SYSTEM SHALL NOT expose an
operational capture service.

IF the host does not provide the capture capability required by the remote action, THEN
THE SYSTEM SHALL fail the action rather than report a successful capture.

**Rationale:** Fleet owners control whether hosts can perform captures, and operators
need an honest outcome when the capability is unavailable.

### REQ-PCAP-003: Exclude application payloads

THE SYSTEM SHALL include only complete, recognized protocol headers in recorded packet
bytes and SHALL NOT include application payload bytes.

IF a header is unknown, malformed, or incomplete, THEN THE SYSTEM SHALL stop recording
that packet at the last complete, recognized header before it.

WHEN recognized encapsulation contains a decodable inner packet, THE SYSTEM SHALL permit
its complete, recognized inner headers under the same exclusion rule.

WHEN a first IP fragment contains a complete recognized transport header, THE SYSTEM
SHALL retain that header; WHEN a later fragment contains only fragment payload after its
network headers, THE SYSTEM SHALL exclude that payload.

**Rationale:** Troubleshooting needs network structure without collecting the
application content carried by that structure.

### REQ-PCAP-004: Bound collection and preserve readable records

IF a requested duration is outside 1 through 120 seconds, THEN THE SYSTEM SHALL reject
the request before collecting packets.

WHEN a valid capture runs, THE SYSTEM SHALL stop collecting when its duration expires,
its packet limit is reached, or the next complete packet record would exceed its
file-size limit.

THE SYSTEM SHALL cap each capture at 2,000,000 recorded packets and 160 MiB of output,
permit lower positive limits, and substitute those ceilings for omitted, non-positive,
or larger limits.

WHEN a requested file-size limit is smaller than the minimum 24-byte capture-file
header, THE SYSTEM SHALL produce the header without packet records.

THE SYSTEM SHALL preserve complete file and packet records and original on-wire packet
lengths in a standard PCAP artifact.

**Rationale:** An investigation must not grow without bounds or produce an unusable file
merely because it reaches a collection limit.

### REQ-PCAP-005: Finish collection when traffic or consumers stop

WHEN a capture starts on an interface with no matching traffic, THE SYSTEM SHALL
acknowledge the started capture without waiting for a first packet and SHALL finish
collection when its duration expires.

WHEN the caller cancels or disconnects, THE SYSTEM SHALL stop the capture and release
its capture resources.

IF a capture consumer stops reading, THEN THE SYSTEM SHALL bound response writes to the
requested duration plus 10 seconds on a transport supporting write deadlines.

**Rationale:** Quiet traffic and abandoned requests must not strand troubleshooting work
or leave captures running indefinitely.

### REQ-PCAP-006: Surface failures before publishing a capture as successful

IF capture setup, transport, local file finalization, or a reported packet read/write
error fails, THEN THE SYSTEM SHALL fail the action and SHALL NOT upload the rejected
capture as a successful artifact.

IF artifact delivery to a required destination fails, THEN THE SYSTEM SHALL fail the
action.

**Rationale:** A successful action must not hide a missing or rejected capture;
operators may otherwise make decisions using evidence they never obtained.

### REQ-PCAP-007: Correlate artifacts and assess capture quality

WHEN an operator supplies a capture identifier, THE SYSTEM SHALL preserve it in the
action result and uploaded artifact metadata.

WHEN no identifier is supplied, THE SYSTEM SHALL generate and return one.

WHEN the action succeeds, THE SYSTEM SHALL report recorded packet count, artifact size,
elapsed capture-request duration, kernel packet-drop count, header-truncation count, and
capture-error count.

THE SYSTEM SHALL associate uploaded captures with their capture identifier, upload
timestamp, and host identity when host identity can be resolved.

**Rationale:** Operators need to find the correct capture and recognize incomplete
evidence; a capture with drops is different from one containing every observed packet.

### REQ-PCAP-008: Deliver transient evidence with bounded recovery

WHEN a completed capture is ready, THE SYSTEM SHALL deliver it with its metadata to the
packet-capture intake before reporting action success.

IF delivery fails with a transport error, request timeout, throttling response, or
server error, THEN THE SYSTEM SHALL retry up to four total attempts while respecting
caller cancellation.

IF delivery is rejected as too large or by another non-retryable client response, THEN
THE SYSTEM SHALL fail that delivery without resending the same artifact.

**Rationale:** A capture represents a past interval that cannot be recreated; transient
delivery failures deserve recovery, and permanent failures need a timely outcome.

### REQ-PCAP-009: Honor configured delivery destinations

WHEN additional delivery destinations are configured, THE SYSTEM SHALL attempt delivery
to each eligible destination using that destination’s credentials.

IF delivery to a reliable destination fails, THEN THE SYSTEM SHALL return that failure
to the caller.

IF delivery to an unreliable destination fails, THEN THE SYSTEM SHALL log the failure
without failing an otherwise successful action.

WHILE log failover is inactive, THE SYSTEM SHALL NOT send captures to the multi-region
failover destination.

**Rationale:** Operators depend on the fleet’s configured routing and reliability
policy, including which destinations can determine success.

### REQ-PCAP-010: Restrict the runner’s access to capture operations

WHERE a capture-only local access channel is configured, THE SYSTEM SHALL make capture
routes available through it and SHALL return not-found responses for routes outside the
capture namespace, including path-normalization escapes.

WHEN the runner has a configured capture-only channel, THE SYSTEM SHALL use it; WHEN
none is configured, THE SYSTEM SHALL use the configured general system-probe channel.

IF the capture-only listener cannot start, THEN THE SYSTEM SHALL log the failure without
preventing other system-probe services from serving requests.

**Rationale:** Deployments need to give the capture runner access to packet capture
without granting access to unrelated privileged operations.

### REQ-PCAP-011: Remove temporary evidence after use

WHEN artifact delivery finishes or fails, THE SYSTEM SHALL attempt to remove the local
temporary capture file before returning from the action.

IF receiving or finalizing the local capture file fails, THEN THE SYSTEM SHALL close the
file and attempt to remove it before returning the failure.

**Rationale:** Troubleshooting must not leave an accumulating collection of sensitive
network evidence on the host after each request.
