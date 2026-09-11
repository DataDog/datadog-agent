// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"crypto/tls"
	"encoding/binary"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	tlstags "github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
)

func TestDarwinPacketAnalyzerDerivesOnlyPacketEvidence(t *testing.T) {
	analyzer := newDarwinPacketAnalyzer(10)
	syn := &layers.TCP{Seq: 100, SYN: true}
	analysis := analyzer.process(1, true, false, syn)
	require.Equal(t, network.OUTGOING, analysis.direction)

	payload := &layers.TCP{
		Seq:       101,
		ACK:       true,
		BaseLayer: layers.BaseLayer{Payload: []byte("payload")},
	}
	analyzer.process(1, true, false, payload)
	analysis = analyzer.process(1, true, false, payload)
	require.Equal(t, uint32(1), analysis.retransmits)

	reset := &layers.TCP{Seq: 200, ACK: true, RST: true}
	analysis = analyzer.process(1, false, false, reset)
	require.True(t, analysis.failure)
	require.Equal(t, network.TCPFailureErrnoConnReset, analysis.failureErrno)
	analysis = analyzer.process(1, false, false, reset)
	require.False(t, analysis.failure)
}

func TestDarwinPacketAnalyzerClassifiesRefusal(t *testing.T) {
	analyzer := newDarwinPacketAnalyzer(10)
	analyzer.process(2, true, false, &layers.TCP{Seq: 10, SYN: true})

	analysis := analyzer.process(2, false, false, &layers.TCP{Seq: 20, ACK: true, RST: true})

	require.True(t, analysis.failure)
	require.Equal(t, network.TCPFailureErrnoConnRefused, analysis.failureErrno)
}

func TestDarwinPrefixAssemblerReordersAndMergesOverlap(t *testing.T) {
	var assembler darwinPrefixAssembler
	assembler.add(105, []byte(" world"))
	assembler.add(100, []byte("hello"))
	assembler.add(103, []byte("lo world!"))

	require.Equal(t, "hello world!", string(assembler.bytes()))
}

// TestDarwinPacketAnalyzerClassifiesOnlyWhenPrefixGrows skips classify on ACKs
// after the HTTP prefix has already been accepted.
func TestDarwinPacketAnalyzerClassifiesOnlyWhenPrefixGrows(t *testing.T) {
	analyzer := newDarwinPacketAnalyzer(10)
	request := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	analysis := analyzer.process(3, true, false, &layers.TCP{
		Seq:       100,
		ACK:       true,
		BaseLayer: layers.BaseLayer{Payload: request},
	})
	require.Equal(t, protocols.HTTP, analysis.protocolStack.Application)

	analyzer.mu.Lock()
	classified := analyzer.flows[3].classifyCount
	analyzer.mu.Unlock()
	require.Equal(t, 1, classified)

	for i := 0; i < 8; i++ {
		analysis = analyzer.process(3, true, false, &layers.TCP{Seq: 100 + uint32(len(request)), ACK: true})
		require.Equal(t, protocols.HTTP, analysis.protocolStack.Application)
	}

	analyzer.mu.Lock()
	require.Equal(t, classified, analyzer.flows[3].classifyCount)
	analyzer.mu.Unlock()
}

func TestClassifyDarwinHTTPPrefixes(t *testing.T) {
	stack, _ := classifyDarwinPrefix([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	require.Equal(t, protocols.HTTP, stack.Application)

	stack, _ = classifyDarwinPrefix([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	require.Equal(t, protocols.HTTP2, stack.Application)

	stack, _ = classifyDarwinPrefix([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: h2c\r\n\r\n"))
	require.Equal(t, protocols.HTTP2, stack.Application)
}

func TestClassifyDarwinTLSClientAndServerHello(t *testing.T) {
	clientStack, clientTags := classifyDarwinPrefix(testDarwinClientHello())
	require.Equal(t, protocols.TLS, clientStack.Encryption)
	require.Equal(
		t,
		tlstags.OfferedTLSVersion12|tlstags.OfferedTLSVersion13,
		clientTags.OfferedVersions,
	)

	serverStack, serverTags := classifyDarwinPrefix(testDarwinServerHello())
	require.Equal(t, protocols.TLS, serverStack.Encryption)
	require.Equal(t, uint16(tls.VersionTLS13), serverTags.ChosenVersion)
	require.Equal(t, uint16(tls.TLS_AES_128_GCM_SHA256), serverTags.CipherSuite)
}

func TestClassifyDarwinTLSRejectsTruncation(t *testing.T) {
	hello := testDarwinClientHello()
	for length := 0; length < len(hello); length++ {
		stack, tags := classifyDarwinPrefix(hello[:length])
		require.NotPanics(t, func() {
			_, _ = classifyDarwinPrefix(hello[:length])
		})
		if length < len(hello) {
			require.Equal(t, protocols.Unknown, stack.Encryption)
			require.True(t, tags.IsEmpty())
		}
	}

	crossRecord := append([]byte(nil), hello...)
	binary.BigEndian.PutUint16(crossRecord[3:5], 4)
	stack, tags := classifyDarwinPrefix(crossRecord)
	require.Equal(t, protocols.Unknown, stack.Encryption)
	require.True(t, tags.IsEmpty())
}

func testDarwinClientHello() []byte {
	body := make([]byte, 0, 64)
	body = binary.BigEndian.AppendUint16(body, tls.VersionTLS12)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0) // session ID
	body = binary.BigEndian.AppendUint16(body, 2)
	body = binary.BigEndian.AppendUint16(body, tls.TLS_AES_128_GCM_SHA256)
	body = append(body, 1, 0) // one null compression method
	extensions := make([]byte, 0, 9)
	extensions = binary.BigEndian.AppendUint16(extensions, tlsSupportedVersion)
	extensions = binary.BigEndian.AppendUint16(extensions, 5)
	extensions = append(extensions, 4)
	extensions = binary.BigEndian.AppendUint16(extensions, tls.VersionTLS13)
	extensions = binary.BigEndian.AppendUint16(extensions, tls.VersionTLS12)
	body = binary.BigEndian.AppendUint16(body, uint16(len(extensions)))
	body = append(body, extensions...)
	return testDarwinTLSRecord(tlsClientHello, body)
}

func testDarwinServerHello() []byte {
	body := make([]byte, 0, 64)
	body = binary.BigEndian.AppendUint16(body, tls.VersionTLS12)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0) // session ID
	body = binary.BigEndian.AppendUint16(body, tls.TLS_AES_128_GCM_SHA256)
	body = append(body, 0) // null compression method
	extensions := make([]byte, 0, 6)
	extensions = binary.BigEndian.AppendUint16(extensions, tlsSupportedVersion)
	extensions = binary.BigEndian.AppendUint16(extensions, 2)
	extensions = binary.BigEndian.AppendUint16(extensions, tls.VersionTLS13)
	body = binary.BigEndian.AppendUint16(body, uint16(len(extensions)))
	body = append(body, extensions...)
	return testDarwinTLSRecord(tlsServerHello, body)
}

func testDarwinTLSRecord(handshakeType byte, body []byte) []byte {
	handshake := []byte{
		handshakeType,
		byte(len(body) >> 16),
		byte(len(body) >> 8),
		byte(len(body)),
	}
	handshake = append(handshake, body...)
	record := []byte{tlsHandshakeRecord, 0x03, 0x03}
	record = binary.BigEndian.AppendUint16(record, uint16(len(handshake)))
	return append(record, handshake...)
}
