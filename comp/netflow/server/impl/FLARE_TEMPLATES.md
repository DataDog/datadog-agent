# NetFlow Template Export in Flares

## Overview

The NetFlow component now automatically exports template information when a flare is created. This helps debug issues where customers send packet captures without the necessary templates.

## What Gets Exported

When a flare is generated, the following files are created in the `netflow/` directory:

### 1. Human-Readable Format (`templates_readable.txt`)

A text file containing all templates with:
- Template metadata (exporter IP, version, observation domain ID, template ID)
- Field definitions with human-readable names
- Field types and lengths
- Support for NetFlow v9, IPFIX, and Options templates

Example:
```
Template Key: 192.168.1.1-9-0-256
Listener: 0.0.0.0:2055
Exporter IP: 192.168.1.1
Version: 9 (NetFlow v9)
Observation Domain ID: 0
Template ID: 256

Type: Data Template
Field Count: 8

Fields:
   1. IPV4_SRC_ADDR                     (Type:     8, Length:   4)
   2. IPV4_DST_ADDR                     (Type:    12, Length:   4)
   3. L4_SRC_PORT                       (Type:     7, Length:   2)
   4. L4_DST_PORT                       (Type:    11, Length:   2)
   ...
```

### 2. JSON Format (`templates.json`)

A machine-readable JSON file containing all template data. Useful for:
- Automated analysis
- Template comparison
- Integration with other tools

### 3. Pcap Format (`templates_*.pcap`)

A pcap file per exporter/observation domain combination, containing one
synthesized Ethernet/IPv4/UDP frame whose payload is the cached NetFlow v9
or IPFIX template packet. The synthetic source IP matches the recorded
exporter address and the destination port matches the protocol default
(2055 for v9, 4739 for IPFIX), so Wireshark's NetFlow dissector caches the
templates against the same key as data records seen in a real capture.

File naming: `templates_{IP}_v{version}_obs{obsdomainid}.pcap`

Example:
- `templates_192.168.1.1_v9_obs0.pcap`
- `templates_10.0.0.5_v10_obs123.pcap`

Each pcap is accompanied by a `*_README.txt` documenting the synthetic
framing and the recommended merge command.

## Use Cases

### 1. Debugging Missing Templates
When customers send Wireshark captures of data flows but don't include the
template packets, merge the template pcap with their capture so Wireshark
can dissect the otherwise-orphan data records:

```
mergecap -a -w merged.pcap templates_<IP>_v<N>_obs<N>.pcap customer.pcap
wireshark merged.pcap
```

`mergecap -a` (append) preserves source order so the synthetic templates
are dissected before the customer's data records regardless of timestamps.

### 2. Template Verification
Check if the agent is receiving and storing templates correctly by inspecting the human-readable format.

### 3. Template Comparison
Compare templates between different environments or time periods using the JSON format.

## Implementation Details

- Templates are collected from all active NetFlow/IPFIX listeners
- sFlow listeners are skipped (they don't use templates)
- Collection has a 5-second timeout per listener
- Pcap frames use synthetic Ethernet/IPv4/UDP headers; only IPv4 exporters
  are exported today (IPv6 exporters are skipped with a warning)
- All formats include the same template data for consistency

## Technical Notes

### Template Sources
Templates are stored in the GoFlow2 in-memory template system and accessed via the `TemplateInterface`:
- `ListTemplates()` - Enumerate all stored templates
- `GetTemplate()` - Retrieve specific template data

### Pcap Format Details
Each pcap contains a single synthesized frame:
- **Link layer**: Ethernet, with locally-administered placeholder MACs
- **Network layer**: IPv4, source = recorded exporter IP, destination =
  127.0.0.1 (synthetic; not a real receiver)
- **Transport layer**: UDP, destination port = 2055 (v9) or 4739 (IPFIX),
  source port = 49152, checksum = 0 (legal in IPv4)
- **Payload**: a complete NetFlow/IPFIX packet
  - **NetFlow v9**: Version 9 header + template FlowSets (ID=0) or options template FlowSets (ID=1)
  - **IPFIX**: Version 10 header + template Sets (ID=2) or options template Sets (ID=3)
  - Both formats include proper headers, padding, and length fields

### Performance Impact
- Templates are collected asynchronously when a flare is generated
- No impact on normal flow processing
- Minimal memory overhead (templates are already in memory)
- Collection continues even if individual templates fail

## Limitations

1. Only active listeners are queried (stopped listeners won't have their templates exported)
2. Templates that have expired from the cache won't be included
3. Only IPv4 exporters produce a pcap today; IPv6 exporters are skipped with a log warning
4. Templates the customer's collector observed but the agent did not (different
   vantage point, NAT rewrites, etc.) cannot be reconstructed from this export

## Future Enhancements

Potential improvements:
- IPv6 exporter support in the pcap path
- Include template statistics (first seen, last seen, usage count)
- Export template history if available
