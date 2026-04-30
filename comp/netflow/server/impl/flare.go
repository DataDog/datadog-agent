// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package serverimpl

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/netflowstate"
	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
)

const (
	pcapMagicMicro    = 0xa1b2c3d4
	pcapLinkEthernet  = 1
	pcapSnapLen       = 65535
	ethTypeIPv4       = 0x0800
	ipProtoUDP        = 17
	netflowV9DstPort  = 2055
	ipfixDstPort      = 4739
	syntheticSrcPort  = 49152
)

// fillFlare adds netflow template information to the flare
func (s *Server) fillFlare(parentCtx context.Context, fb flaretypes.FlareBuilder) error {
	if !s.running {
		s.logger.Debug("NetFlow server not running, skipping template export for flare")
		return nil
	}

	s.logger.Debug("Collecting NetFlow templates for flare")

	// Collect templates from all listeners
	allTemplates := make(map[string]*TemplateInfo)

	for _, listener := range s.listeners {
		if listener == nil || listener.flowState == nil {
			continue
		}

		// Check if this is a NetFlow/IPFIX listener (not sFlow)
		state, ok := listener.flowState.State.(*netflowstate.StateNetFlow)
		if !ok {
			s.logger.Debugf("Skipping non-NetFlow listener on %s", listener.config.Addr())
			continue
		}

		s.logger.Debugf("Collecting templates from listener on %s", listener.config.Addr())

		// Get templates from this listener
		ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
		templatesChan := make(chan *templates.TemplateKey, 100)

		// Start collecting templates
		go func() {
			defer close(templatesChan)
			if err := state.TemplateSystem.ListTemplates(ctx, templatesChan); err != nil {
				s.logger.Errorf("Error listing templates: %v", err)
			}
		}()

		for key := range templatesChan {
			if key == nil {
				// goflow2's memory driver signals end-of-stream with a nil key.
				break
			}
			templateData, err := state.TemplateSystem.GetTemplate(ctx, key)
			if err != nil {
				s.logger.Warnf("Error getting template %s: %v", key.String(), err)
				continue
			}

			// Store template info
			allTemplates[key.String()] = &TemplateInfo{
				Key:          key,
				TemplateData: templateData,
				ListenerAddr: listener.config.Addr(),
			}
		}

		cancel()
	}

	if len(allTemplates) == 0 {
		s.logger.Info("No NetFlow templates found to export")
		return fb.AddFile("netflow/templates_summary.txt", []byte("No NetFlow templates found.\n"))
	}

	s.logger.Infof("Found %d NetFlow templates to export", len(allTemplates))

	// Export human-readable format
	if err := s.exportHumanReadableTemplates(fb, allTemplates); err != nil {
		s.logger.Errorf("Error exporting human-readable templates: %v", err)
	}

	// Export JSON format
	if err := s.exportJSONTemplates(fb, allTemplates); err != nil {
		s.logger.Errorf("Error exporting JSON templates: %v", err)
	}

	// Export pcap format (mergeable with customer pcaps for offline decoding)
	if err := s.exportPcapTemplates(fb, allTemplates); err != nil {
		s.logger.Errorf("Error exporting pcap templates: %v", err)
	}

	return nil
}

// TemplateInfo holds information about a single template
type TemplateInfo struct {
	Key          *templates.TemplateKey
	TemplateData interface{}
	ListenerAddr string
}

// exportHumanReadableTemplates exports templates in a human-readable text format
func (s *Server) exportHumanReadableTemplates(fb flaretypes.FlareBuilder, allTemplates map[string]*TemplateInfo) error {
	var buf bytes.Buffer

	buf.WriteString("NetFlow Template Export\n")
	buf.WriteString("=======================\n\n")
	buf.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().UTC().Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("Total Templates: %d\n\n", len(allTemplates)))

	for keyStr, info := range allTemplates {
		buf.WriteString("----------------------------------------\n")
		buf.WriteString(fmt.Sprintf("Template Key: %s\n", keyStr))
		buf.WriteString(fmt.Sprintf("Listener: %s\n", info.ListenerAddr))
		buf.WriteString(fmt.Sprintf("Exporter IP: %s\n", info.Key.TemplateKey))
		buf.WriteString(fmt.Sprintf("Version: %d (", info.Key.Version))
		if info.Key.Version == 9 {
			buf.WriteString("NetFlow v9)\n")
		} else if info.Key.Version == 10 {
			buf.WriteString("IPFIX)\n")
		} else {
			buf.WriteString("Unknown)\n")
		}
		buf.WriteString(fmt.Sprintf("Observation Domain ID: %d\n", info.Key.ObsDomainId))
		buf.WriteString(fmt.Sprintf("Template ID: %d\n\n", info.Key.TemplateId))

		// Format the template data based on its type
		switch tmpl := info.TemplateData.(type) {
		case netflow.TemplateRecord:
			buf.WriteString("Type: Data Template\n")
			buf.WriteString(fmt.Sprintf("Field Count: %d\n\n", tmpl.FieldCount))
			buf.WriteString("Fields:\n")
			for i, field := range tmpl.Fields {
				fieldName := getFieldName(field.Type, info.Key.Version)
				buf.WriteString(fmt.Sprintf("  %2d. %-35s (Type: %5d, Length: %3d", i+1, fieldName, field.Type, field.Length))
				if field.PenProvided {
					buf.WriteString(fmt.Sprintf(", PEN: %d", field.Pen))
				}
				buf.WriteString(")\n")
			}

		case netflow.NFv9OptionsTemplateRecord:
			buf.WriteString("Type: NetFlow v9 Options Template\n")
			buf.WriteString(fmt.Sprintf("Scope Length: %d\n", tmpl.ScopeLength))
			buf.WriteString(fmt.Sprintf("Option Length: %d\n\n", tmpl.OptionLength))

			buf.WriteString("Scope Fields:\n")
			for i, field := range tmpl.Scopes {
				fieldName := getFieldName(field.Type, info.Key.Version)
				buf.WriteString(fmt.Sprintf("  %2d. %-35s (Type: %5d, Length: %3d)\n", i+1, fieldName, field.Type, field.Length))
			}

			buf.WriteString("\nOption Fields:\n")
			for i, field := range tmpl.Options {
				fieldName := getFieldName(field.Type, info.Key.Version)
				buf.WriteString(fmt.Sprintf("  %2d. %-35s (Type: %5d, Length: %3d)\n", i+1, fieldName, field.Type, field.Length))
			}

		case netflow.IPFIXOptionsTemplateRecord:
			buf.WriteString("Type: IPFIX Options Template\n")
			buf.WriteString(fmt.Sprintf("Field Count: %d\n", tmpl.FieldCount))
			buf.WriteString(fmt.Sprintf("Scope Field Count: %d\n\n", tmpl.ScopeFieldCount))

			buf.WriteString("Scope Fields:\n")
			for i, field := range tmpl.Scopes {
				fieldName := getFieldName(field.Type, info.Key.Version)
				buf.WriteString(fmt.Sprintf("  %2d. %-35s (Type: %5d, Length: %3d", i+1, fieldName, field.Type, field.Length))
				if field.PenProvided {
					buf.WriteString(fmt.Sprintf(", PEN: %d", field.Pen))
				}
				buf.WriteString(")\n")
			}

			buf.WriteString("\nOption Fields:\n")
			for i, field := range tmpl.Options {
				fieldName := getFieldName(field.Type, info.Key.Version)
				buf.WriteString(fmt.Sprintf("  %2d. %-35s (Type: %5d, Length: %3d", i+1, fieldName, field.Type, field.Length))
				if field.PenProvided {
					buf.WriteString(fmt.Sprintf(", PEN: %d", field.Pen))
				}
				buf.WriteString(")\n")
			}

		default:
			buf.WriteString(fmt.Sprintf("Type: Unknown (%T)\n", tmpl))
		}

		buf.WriteString("\n")
	}

	return fb.AddFile("netflow/templates_readable.txt", buf.Bytes())
}

// exportJSONTemplates exports templates in JSON format for programmatic access
func (s *Server) exportJSONTemplates(fb flaretypes.FlareBuilder, allTemplates map[string]*TemplateInfo) error {
	jsonData := make([]map[string]interface{}, 0, len(allTemplates))

	for keyStr, info := range allTemplates {
		entry := map[string]interface{}{
			"template_key":          keyStr,
			"listener_addr":         info.ListenerAddr,
			"exporter_ip":           info.Key.TemplateKey,
			"version":               info.Key.Version,
			"observation_domain_id": info.Key.ObsDomainId,
			"template_id":           info.Key.TemplateId,
		}

		// Add template-specific data
		switch tmpl := info.TemplateData.(type) {
		case netflow.TemplateRecord:
			entry["template_type"] = "data"
			entry["field_count"] = tmpl.FieldCount
			fields := make([]map[string]interface{}, len(tmpl.Fields))
			for i, field := range tmpl.Fields {
				fields[i] = map[string]interface{}{
					"type":         field.Type,
					"name":         getFieldName(field.Type, info.Key.Version),
					"length":       field.Length,
					"pen_provided": field.PenProvided,
					"pen":          field.Pen,
				}
			}
			entry["fields"] = fields

		case netflow.NFv9OptionsTemplateRecord:
			entry["template_type"] = "nfv9_options"
			entry["scope_length"] = tmpl.ScopeLength
			entry["option_length"] = tmpl.OptionLength

			scopes := make([]map[string]interface{}, len(tmpl.Scopes))
			for i, field := range tmpl.Scopes {
				scopes[i] = map[string]interface{}{
					"type":   field.Type,
					"name":   getFieldName(field.Type, info.Key.Version),
					"length": field.Length,
				}
			}
			entry["scope_fields"] = scopes

			options := make([]map[string]interface{}, len(tmpl.Options))
			for i, field := range tmpl.Options {
				options[i] = map[string]interface{}{
					"type":   field.Type,
					"name":   getFieldName(field.Type, info.Key.Version),
					"length": field.Length,
				}
			}
			entry["option_fields"] = options

		case netflow.IPFIXOptionsTemplateRecord:
			entry["template_type"] = "ipfix_options"
			entry["field_count"] = tmpl.FieldCount
			entry["scope_field_count"] = tmpl.ScopeFieldCount

			scopes := make([]map[string]interface{}, len(tmpl.Scopes))
			for i, field := range tmpl.Scopes {
				scopes[i] = map[string]interface{}{
					"type":         field.Type,
					"name":         getFieldName(field.Type, info.Key.Version),
					"length":       field.Length,
					"pen_provided": field.PenProvided,
					"pen":          field.Pen,
				}
			}
			entry["scope_fields"] = scopes

			options := make([]map[string]interface{}, len(tmpl.Options))
			for i, field := range tmpl.Options {
				options[i] = map[string]interface{}{
					"type":         field.Type,
					"name":         getFieldName(field.Type, info.Key.Version),
					"length":       field.Length,
					"pen_provided": field.PenProvided,
					"pen":          field.Pen,
				}
			}
			entry["option_fields"] = options
		}

		jsonData = append(jsonData, entry)
	}

	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal templates to JSON: %w", err)
	}

	return fb.AddFile("netflow/templates.json", jsonBytes)
}

// exportPcapTemplates exports templates as pcap files containing synthesized
// UDP/IP/Ethernet frames carrying NetFlow/IPFIX template packets. The synthetic
// packets use the recorded exporter IP as the source address, so the resulting
// pcap can be merged with a customer capture via `mergecap` to populate
// Wireshark's per-exporter template cache and decode otherwise-orphan data
// records.
func (s *Server) exportPcapTemplates(fb flaretypes.FlareBuilder, allTemplates map[string]*TemplateInfo) error {
	// Group templates by exporter and version
	type exporterKey struct {
		ExporterIP  string
		Version     uint16
		ObsDomainID uint32
	}

	exporters := make(map[exporterKey][]*TemplateInfo)
	for _, info := range allTemplates {
		key := exporterKey{
			ExporterIP:  info.Key.TemplateKey,
			Version:     info.Key.Version,
			ObsDomainID: info.Key.ObsDomainId,
		}
		exporters[key] = append(exporters[key], info)
	}

	for expKey, templates := range exporters {
		srcIP := net.ParseIP(expKey.ExporterIP)
		if srcIP == nil {
			s.logger.Warnf("Skipping pcap export: exporter address %q is not a parseable IP", expKey.ExporterIP)
			continue
		}
		if srcIP.To4() == nil {
			// IPv4-only for now; IPv6 would require an IPv6 header path.
			s.logger.Warnf("Skipping pcap export for non-IPv4 exporter %s (IPv6 templates pcap not yet supported)", expKey.ExporterIP)
			continue
		}

		var payload bytes.Buffer
		var dstPort uint16
		var err error
		switch expKey.Version {
		case 9:
			err = encodeNetFlowV9Templates(&payload, expKey.ObsDomainID, templates)
			dstPort = netflowV9DstPort
		case 10:
			err = encodeIPFIXTemplates(&payload, expKey.ObsDomainID, templates)
			dstPort = ipfixDstPort
		default:
			s.logger.Warnf("Unknown NetFlow version %d for exporter %s", expKey.Version, expKey.ExporterIP)
			continue
		}
		if err != nil {
			s.logger.Errorf("Error encoding templates for %s: %v", expKey.ExporterIP, err)
			continue
		}

		pcapBytes, err := buildTemplatesPcap(payload.Bytes(), srcIP.To4(), dstPort, time.Now())
		if err != nil {
			s.logger.Errorf("Error building pcap for %s: %v", expKey.ExporterIP, err)
			continue
		}

		sanitizedIP := filepath.Base(expKey.ExporterIP)
		filename := fmt.Sprintf("netflow/templates_%s_v%d_obs%d.pcap", sanitizedIP, expKey.Version, expKey.ObsDomainID)
		if err := fb.AddFileWithoutScrubbing(filename, pcapBytes); err != nil {
			return err
		}

		readmePath := fmt.Sprintf("netflow/templates_%s_v%d_obs%d_README.txt", sanitizedIP, expKey.Version, expKey.ObsDomainID)
		readme := fmt.Sprintf(`NetFlow Template Pcap
======================

Exporter IP: %s
Version: %d (%s)
Observation Domain ID: %d
Template Count: %d

This pcap contains one synthesized UDP packet carrying the templates the
agent had cached for this exporter at flare-collection time. It is NOT a
real capture; the Ethernet/IPv4/UDP framing is synthetic. The source IP
matches the exporter so Wireshark's NetFlow dissector caches the templates
against the same key as data records seen in a real capture.

Decoding a customer pcap that lacks templates:

  mergecap -a -w merged.pcap %s customer.pcap
  wireshark merged.pcap

Use -a (append) so the synthetic templates are dissected before the
customer's data records regardless of timestamps.

Caveats:
- Only templates the agent observed are included. Templates the customer's
  collector saw but the agent did not (different vantage point, NAT, etc.)
  cannot be reconstructed here.
- If the customer's exporter address differs from the one above (e.g. NAT
  rewrites between agent and customer's collector), Wireshark will not
  match the templates against the customer's data records.
`, expKey.ExporterIP, expKey.Version, versionName(expKey.Version), expKey.ObsDomainID, len(templates), filepath.Base(filename))

		if err := fb.AddFile(readmePath, []byte(readme)); err != nil {
			return err
		}
	}

	return nil
}

// maxIPv4UDPPayload is the largest UDP payload that fits in a single IPv4
// datagram (65535 - 20 IP header - 8 UDP header). NetFlow exporters fragment
// large template sets across packets in real life; we don't, so a wildly
// over-sized template set has to be reported instead of silently truncated by
// the uint16 length fields.
const maxIPv4UDPPayload = 65535 - 20 - 8

// buildTemplatesPcap returns a complete pcap byte slice containing one
// Ethernet/IPv4/UDP frame whose UDP payload is the supplied NetFlow/IPFIX
// template packet. srcIP must be a 4-byte IPv4 address.
func buildTemplatesPcap(payload []byte, srcIP net.IP, dstPort uint16, ts time.Time) ([]byte, error) {
	if len(srcIP) != net.IPv4len {
		return nil, fmt.Errorf("srcIP must be IPv4 (got %d bytes)", len(srcIP))
	}
	if len(payload) > maxIPv4UDPPayload {
		return nil, fmt.Errorf("template payload %d bytes exceeds max IPv4 UDP payload %d", len(payload), maxIPv4UDPPayload)
	}

	frame := buildEthernetIPv4UDP(payload, srcIP, dstPort)

	var buf bytes.Buffer
	// pcap global header
	if err := binary.Write(&buf, binary.LittleEndian, struct {
		Magic    uint32
		Major    uint16
		Minor    uint16
		ThisZone int32
		SigFigs  uint32
		SnapLen  uint32
		LinkType uint32
	}{
		Magic: pcapMagicMicro, Major: 2, Minor: 4,
		SnapLen: pcapSnapLen, LinkType: pcapLinkEthernet,
	}); err != nil {
		return nil, err
	}

	// pcap record header + frame
	if err := binary.Write(&buf, binary.LittleEndian, struct {
		TsSec   uint32
		TsUsec  uint32
		InclLen uint32
		OrigLen uint32
	}{
		TsSec:   uint32(ts.Unix()),
		TsUsec:  uint32(ts.Nanosecond() / 1000),
		InclLen: uint32(len(frame)),
		OrigLen: uint32(len(frame)),
	}); err != nil {
		return nil, err
	}
	buf.Write(frame)
	return buf.Bytes(), nil
}

// buildEthernetIPv4UDP returns a complete frame: Ethernet | IPv4 | UDP | payload.
// Destination address is loopback (127.0.0.1); MAC addresses are locally
// administered placeholders. The UDP checksum is left zero (legal for IPv4).
func buildEthernetIPv4UDP(payload []byte, srcIP net.IP, dstPort uint16) []byte {
	const ethLen, ipLen, udpLen = 14, 20, 8
	frame := make([]byte, ethLen+ipLen+udpLen+len(payload))

	// Ethernet
	copy(frame[0:6], []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x02})  // dst MAC
	copy(frame[6:12], []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}) // src MAC
	binary.BigEndian.PutUint16(frame[12:14], ethTypeIPv4)

	ip := frame[ethLen : ethLen+ipLen]
	udp := frame[ethLen+ipLen : ethLen+ipLen+udpLen]
	udpAndPayload := frame[ethLen+ipLen:]

	// IPv4
	ip[0] = 0x45 // version 4, IHL 5
	ip[1] = 0    // DSCP/ECN
	binary.BigEndian.PutUint16(ip[2:4], uint16(ipLen+udpLen+len(payload))) // total length
	binary.BigEndian.PutUint16(ip[4:6], 0)                                 // identification
	binary.BigEndian.PutUint16(ip[6:8], 0x4000)                            // flags=DF, fragment offset 0
	ip[8] = 64                                                             // TTL
	ip[9] = ipProtoUDP                                                     // protocol
	binary.BigEndian.PutUint16(ip[10:12], 0)                               // checksum (computed below)
	copy(ip[12:16], srcIP)
	copy(ip[16:20], net.IPv4(127, 0, 0, 1).To4())
	binary.BigEndian.PutUint16(ip[10:12], onesComplementChecksum(ip))

	// UDP
	binary.BigEndian.PutUint16(udp[0:2], syntheticSrcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen+len(payload)))
	binary.BigEndian.PutUint16(udp[6:8], 0) // checksum: 0 = "not computed", legal in IPv4

	copy(udpAndPayload[udpLen:], payload)
	return frame
}

// onesComplementChecksum is the standard 16-bit one's-complement sum used by IPv4.
func onesComplementChecksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// writeBinary is a helper to write binary data and check errors
func writeBinary(buf *bytes.Buffer, data interface{}) error {
	return binary.Write(buf, binary.BigEndian, data)
}

// encodeNetFlowV9Templates encodes NetFlow v9 templates into a binary packet
func encodeNetFlowV9Templates(buf *bytes.Buffer, obsDomainID uint32, templates []*TemplateInfo) error {
	// NetFlow v9 packet header
	if err := writeBinary(buf, uint16(9)); err != nil {
		return err
	}
	if err := writeBinary(buf, uint16(len(templates))); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(time.Now().Unix())); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(time.Now().Unix())); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(0)); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(obsDomainID)); err != nil {
		return err
	}

	for _, info := range templates {
		switch tmpl := info.TemplateData.(type) {
		case netflow.TemplateRecord:
			// Template FlowSet header
			flowSetStart := buf.Len()
			if err := writeBinary(buf, uint16(0)); err != nil {
				return err
			}
			lengthPos := buf.Len()
			if err := writeBinary(buf, uint16(0)); err != nil {
				return err
			}

			// Template Record
			if err := writeBinary(buf, tmpl.TemplateId); err != nil {
				return err
			}
			if err := writeBinary(buf, tmpl.FieldCount); err != nil {
				return err
			}

			for _, field := range tmpl.Fields {
				if err := writeBinary(buf, field.Type); err != nil {
					return err
				}
				if err := writeBinary(buf, field.Length); err != nil {
					return err
				}
			}

			// Update length
			flowSetLen := buf.Len() - flowSetStart
			// Pad to 4-byte boundary
			for flowSetLen%4 != 0 {
				buf.WriteByte(0)
				flowSetLen++
			}

			// Write the actual length
			oldBytes := buf.Bytes()
			binary.BigEndian.PutUint16(oldBytes[lengthPos:], uint16(flowSetLen))

		case netflow.NFv9OptionsTemplateRecord:
			// Options Template FlowSet header
			flowSetStart := buf.Len()
			if err := writeBinary(buf, uint16(1)); err != nil {
				return err
			}
			lengthPos := buf.Len()
			if err := writeBinary(buf, uint16(0)); err != nil {
				return err
			}

			// Options Template Record
			if err := writeBinary(buf, tmpl.TemplateId); err != nil {
				return err
			}
			if err := writeBinary(buf, tmpl.ScopeLength); err != nil {
				return err
			}
			if err := writeBinary(buf, tmpl.OptionLength); err != nil {
				return err
			}

			for _, field := range tmpl.Scopes {
				if err := writeBinary(buf, field.Type); err != nil {
					return err
				}
				if err := writeBinary(buf, field.Length); err != nil {
					return err
				}
			}

			for _, field := range tmpl.Options {
				if err := writeBinary(buf, field.Type); err != nil {
					return err
				}
				if err := writeBinary(buf, field.Length); err != nil {
					return err
				}
			}

			// Update length
			flowSetLen := buf.Len() - flowSetStart
			// Pad to 4-byte boundary
			for flowSetLen%4 != 0 {
				buf.WriteByte(0)
				flowSetLen++
			}

			oldBytes := buf.Bytes()
			binary.BigEndian.PutUint16(oldBytes[lengthPos:], uint16(flowSetLen))
		}
	}

	return nil
}

// encodeIPFIXTemplates encodes IPFIX templates into a binary packet
func encodeIPFIXTemplates(buf *bytes.Buffer, obsDomainID uint32, templates []*TemplateInfo) error {
	// IPFIX packet header
	if err := writeBinary(buf, uint16(10)); err != nil {
		return err
	}
	lengthPos := buf.Len()
	if err := writeBinary(buf, uint16(0)); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(time.Now().Unix())); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(0)); err != nil {
		return err
	}
	if err := writeBinary(buf, uint32(obsDomainID)); err != nil {
		return err
	}

	bufStart := buf.Len() - 16 // start of the IPFIX header we just wrote

	for _, info := range templates {
		switch tmpl := info.TemplateData.(type) {
		case netflow.TemplateRecord:
			// Template Set header
			setStart := buf.Len()
			if err := writeBinary(buf, uint16(2)); err != nil {
				return err
			}
			setLengthPos := buf.Len()
			if err := writeBinary(buf, uint16(0)); err != nil {
				return err
			}

			// Template Record
			if err := writeBinary(buf, tmpl.TemplateId); err != nil {
				return err
			}
			if err := writeBinary(buf, tmpl.FieldCount); err != nil {
				return err
			}

			for _, field := range tmpl.Fields {
				fieldType := field.Type
				if field.PenProvided {
					fieldType |= 0x8000 // Set enterprise bit
				}
				if err := writeBinary(buf, fieldType); err != nil {
					return err
				}
				if err := writeBinary(buf, field.Length); err != nil {
					return err
				}
				if field.PenProvided {
					if err := writeBinary(buf, field.Pen); err != nil {
						return err
					}
				}
			}

			// Update set length
			setLen := buf.Len() - setStart
			// Pad to 4-byte boundary
			for setLen%4 != 0 {
				buf.WriteByte(0)
				setLen++
			}

			oldBytes := buf.Bytes()
			binary.BigEndian.PutUint16(oldBytes[setLengthPos:], uint16(setLen))

		case netflow.IPFIXOptionsTemplateRecord:
			// Options Template Set header
			setStart := buf.Len()
			if err := writeBinary(buf, uint16(3)); err != nil {
				return err
			}
			setLengthPos := buf.Len()
			if err := writeBinary(buf, uint16(0)); err != nil {
				return err
			}

			// Options Template Record
			if err := writeBinary(buf, tmpl.TemplateId); err != nil {
				return err
			}
			if err := writeBinary(buf, tmpl.FieldCount); err != nil {
				return err
			}
			if err := writeBinary(buf, tmpl.ScopeFieldCount); err != nil {
				return err
			}

			// Write scope fields
			for _, field := range tmpl.Scopes {
				fieldType := field.Type
				if field.PenProvided {
					fieldType |= 0x8000
				}
				if err := writeBinary(buf, fieldType); err != nil {
					return err
				}
				if err := writeBinary(buf, field.Length); err != nil {
					return err
				}
				if field.PenProvided {
					if err := writeBinary(buf, field.Pen); err != nil {
						return err
					}
				}
			}

			// Write option fields
			for _, field := range tmpl.Options {
				fieldType := field.Type
				if field.PenProvided {
					fieldType |= 0x8000
				}
				if err := writeBinary(buf, fieldType); err != nil {
					return err
				}
				if err := writeBinary(buf, field.Length); err != nil {
					return err
				}
				if field.PenProvided {
					if err := writeBinary(buf, field.Pen); err != nil {
						return err
					}
				}
			}

			// Update set length
			setLen := buf.Len() - setStart
			// Pad to 4-byte boundary
			for setLen%4 != 0 {
				buf.WriteByte(0)
				setLen++
			}

			oldBytes := buf.Bytes()
			binary.BigEndian.PutUint16(oldBytes[setLengthPos:], uint16(setLen))
		}
	}

	totalLen := buf.Len() - bufStart
	oldBytes := buf.Bytes()
	binary.BigEndian.PutUint16(oldBytes[lengthPos:], uint16(totalLen))

	return nil
}

// getFieldName returns a human-readable name for a field type
func getFieldName(fieldType uint16, version uint16) string {
	if version == 9 {
		return netflow.NFv9TypeToString(fieldType)
	} else if version == 10 {
		return netflow.IPFIXTypeToString(fieldType)
	}
	return fmt.Sprintf("UNKNOWN_%d", fieldType)
}

// versionName returns a human-readable version name
func versionName(version uint16) string {
	if version == 9 {
		return "NetFlow v9"
	} else if version == 10 {
		return "IPFIX"
	}
	return "Unknown"
}
