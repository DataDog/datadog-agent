// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnmexporter

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// encodeAndSubmit batches connections, encodes them as CollectorConnections
// protobuf payloads, compresses with zstd, and submits via the forwarder.
func (e *cnmExporter) encodeAndSubmit(conns []network.ConnectionStats, hostname string, groupID int32) error {
	if e.forwarder == nil {
		e.logger.Warn("CNM exporter: no forwarder configured, dropping payload")
		return nil
	}

	maxPerMsg := e.cfg.MaxConnsPerMessage
	numBatches := len(conns) / maxPerMsg
	if len(conns)%maxPerMsg > 0 {
		numBatches++
	}

	agentVersion, _ := version.Agent()

	for batchIdx := 0; batchIdx < numBatches; batchIdx++ {
		start := batchIdx * maxPerMsg
		end := start + maxPerMsg
		if end > len(conns) {
			end = len(conns)
		}
		batch := conns[start:end]

		payload, err := encodeBatch(batch, hostname, groupID, int32(numBatches))
		if err != nil {
			return fmt.Errorf("encode batch %d: %w", batchIdx, err)
		}

		compressed, err := compressPayload(payload)
		if err != nil {
			return fmt.Errorf("compress batch %d: %w", batchIdx, err)
		}

		extraHeaders := make(http.Header)
		extraHeaders.Set(headers.HostHeader, hostname)
		extraHeaders.Set(headers.ProcessVersionHeader, agentVersion.GetNumber())
		extraHeaders.Set(headers.ContentTypeHeader, headers.ProtobufContentType)
		extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(time.Now().Unix())))

		forwarderPayload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressed})
		responses, err := e.forwarder.SubmitConnectionChecks(forwarderPayload, extraHeaders)
		if err != nil {
			return fmt.Errorf("submit batch %d: %w", batchIdx, err)
		}
		// Drain response channel so the forwarder doesn't block on a full chan.
		if responses != nil {
			for resp := range responses {
				_ = resp
			}
		}
	}

	return nil
}

// encodeBatch serializes a batch of connections into a CollectorConnections protobuf message.
func encodeBatch(conns []network.ConnectionStats, hostname string, groupID, groupSize int32) ([]byte, error) {
	builder := model.NewCollectorConnectionsBuilder(io.Discard)

	builder.SetHostName(hostname)
	builder.SetGroupId(groupID)
	builder.SetGroupSize(groupSize)

	for i := range conns {
		conn := &conns[i]
		builder.AddConnections(func(cb *model.ConnectionBuilder) {
			encodeConnection(cb, conn)
		})
	}

	var buf bytes.Buffer
	builder.Reset(&buf)
	builder.SetHostName(hostname)
	builder.SetGroupId(groupID)
	builder.SetGroupSize(groupSize)

	for i := range conns {
		conn := &conns[i]
		builder.AddConnections(func(cb *model.ConnectionBuilder) {
			encodeConnection(cb, conn)
		})
	}

	return buf.Bytes(), nil
}

// encodeConnection writes a single connection's fields into a ConnectionBuilder.
func encodeConnection(cb *model.ConnectionBuilder, conn *network.ConnectionStats) {
	cb.SetPid(int32(conn.Pid))

	cb.SetLaddr(func(ab *model.AddrBuilder) {
		ab.SetIp(conn.Source.String())
		ab.SetPort(int32(conn.SPort))
	})
	cb.SetRaddr(func(ab *model.AddrBuilder) {
		ab.SetIp(conn.Dest.String())
		ab.SetPort(int32(conn.DPort))
	})

	switch conn.Family {
	case network.AFINET:
		cb.SetFamily(uint64(model.ConnectionFamily_v4))
	case network.AFINET6:
		cb.SetFamily(uint64(model.ConnectionFamily_v6))
	}

	switch conn.Type {
	case network.TCP:
		cb.SetType(uint64(model.ConnectionType_tcp))
	case network.UDP:
		cb.SetType(uint64(model.ConnectionType_udp))
	}

	switch conn.Direction {
	case network.INCOMING:
		cb.SetDirection(uint64(model.ConnectionDirection_incoming))
	case network.OUTGOING:
		cb.SetDirection(uint64(model.ConnectionDirection_outgoing))
	case network.LOCAL:
		cb.SetDirection(uint64(model.ConnectionDirection_local))
	case network.NONE:
		cb.SetDirection(uint64(model.ConnectionDirection_none))
	}

	cb.SetLastBytesSent(conn.Monotonic.SentBytes)
	cb.SetLastBytesReceived(conn.Monotonic.RecvBytes)
	cb.SetLastPacketsSent(conn.Monotonic.SentPackets)
	cb.SetLastPacketsReceived(conn.Monotonic.RecvPackets)
	cb.SetLastRetransmits(uint32(conn.Monotonic.Retransmits))
	cb.SetLastTcpEstablished(uint32(conn.Monotonic.TCPEstablished))
	cb.SetLastTcpClosed(uint32(conn.Monotonic.TCPClosed))

	cb.SetRtt(conn.RTT)
	cb.SetRttVar(conn.RTTVar)

	cb.SetNetNS(conn.NetNS)
	cb.SetIntraHost(conn.IntraHost)

	if conn.IPTranslation != nil {
		cb.SetIpTranslation(func(itb *model.IPTranslationBuilder) {
			itb.SetReplSrcIP(conn.IPTranslation.ReplSrcIP.String())
			itb.SetReplDstIP(conn.IPTranslation.ReplDstIP.String())
			itb.SetReplSrcPort(int32(conn.IPTranslation.ReplSrcPort))
			itb.SetReplDstPort(int32(conn.IPTranslation.ReplDstPort))
		})
	}
}

func compressPayload(data []byte) ([]byte, error) {
	compressed, err := zstd.Compress(nil, data)
	if err != nil {
		return nil, fmt.Errorf("zstd compress: %w", err)
	}
	return compressed, nil
}
