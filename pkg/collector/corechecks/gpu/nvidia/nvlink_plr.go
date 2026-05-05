// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	tlvTypeEnd = 0x0
	tlvTypeOp  = 0x1
	tlvTypeReg = 0x3

	opTLVLenDwords               = 4
	opTLVClassReg                = 1
	opTLVMethodQuery             = 1
	regTLVHeaderLenDwords        = 1
	endTLVLenDwords              = 1
	dwordSizeBytes               = 4
	ppcntRegID                   = 0x5008
	ppcntGroupPLR                = 0x22
	ppcntSizeBytes               = 256
	opTLVRequestBit       uint32 = 0
)

var plrCounterFields = []string{
	"nvlink.plr.rx.codes",
	"nvlink.plr.rx.code_err",
	"nvlink.plr.rx.uncorrectable_code",
	"nvlink.plr.tx.codes",
	"nvlink.plr.tx.retry_codes",
	"nvlink.plr.tx.retry_events",
	"nvlink.plr.tx.sync_events",
	"nvlink.plr.codes_loss",
	"nvlink.plr.tx.retry_events_within_t_sec_max",
}

type nvlinkPLRCollector struct {
	device ddnvml.Device
	ports  []int
}

func newNVLinkPLRCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	c := &nvlinkPLRCollector{
		device: device,
	}

	ports, err := getSupportedNvlinkPorts(device, c.getPortMetrics)
	if err != nil {
		return nil, err
	}

	c.ports = ports

	return c, nil
}

func (c *nvlinkPLRCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *nvlinkPLRCollector) Name() CollectorName {
	return nvlinkPLR
}

func (c *nvlinkPLRCollector) Collect() ([]*Metric, error) {
	var (
		allMetrics []*Metric
		multiErr   error
	)

	for _, port := range c.ports {
		metrics, err := c.getPortMetrics(port)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("get port metrics for port %d: %w", port, err))
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, multiErr
}

func (c *nvlinkPLRCollector) getPortMetrics(port int) ([]*Metric, error) {
	var allMetrics []*Metric
	counters, err := c.readPortCounters(port)
	if err != nil {
		return nil, fmt.Errorf("read port counters: %w", err)
	}

	var multiErr error
	for _, field := range plrCounterFields {
		value, found := counters[field]
		if !found {
			multiErr = multierror.Append(multiErr, fmt.Errorf("missing PLR counter %q for port %d", field, port))
			continue
		}

		allMetrics = append(allMetrics, &Metric{
			Name:  field,
			Value: float64(value),
			Type:  metrics.GaugeType,
			Tags: []string{
				nvlinkPortTag(port),
			},
			Priority: Medium,
		})
	}

	return allMetrics, multiErr
}

func (c *nvlinkPLRCollector) readPortCounters(port int) (map[string]uint64, error) {
	tlvBytes := createPPCNTTLVByteArray(ppcntGroupPLR, uint32(port))
	var prm nvml.PRMTLV_v1
	if len(tlvBytes) > len(prm.InData) {
		return nil, fmt.Errorf("PPCNT TLV payload too large: %d", len(tlvBytes))
	}

	prm.DataSize = uint32(len(tlvBytes))
	copy(prm.InData[:], tlvBytes)

	if err := c.device.ReadWritePRM_v1(&prm); err != nil {
		return nil, fmt.Errorf("issue raw PRM query: %w", err)
	}

	// InData and outData are a C union in nvmlPRMTLV_v1_t; the NVML API
	// writes the response back into the same buffer that held the request.
	return unpackTLV(prm.InData[:])
}

func createPPCNTTLVByteArray(group, port uint32) []byte {
	return packTLV(ppcntRegID, ppcntSizeBytes, createPPCNTByteArray(group, port))
}

func createPPCNTByteArray(group, port uint32) []byte {
	payload := make([]byte, ppcntSizeBytes)
	ppcntVal := (group & 0x3F) | (port << 16)
	binary.BigEndian.PutUint32(payload[0:dwordSizeBytes], ppcntVal)
	return payload
}

func packTLV(regID uint32, regSize int, regPayload []byte) []byte {
	ret := make([]byte, 0, (opTLVLenDwords+regTLVHeaderLenDwords+endTLVLenDwords)*dwordSizeBytes+regSize)
	ret = append(ret, packOpTLV(regID)...)
	ret = append(ret, packDWord(makeTLVHeader(tlvTypeReg, uint32(regSize/dwordSizeBytes+regTLVHeaderLenDwords)))...)
	if regPayload != nil {
		ret = append(ret, regPayload...)
	} else {
		ret = append(ret, make([]byte, regSize)...)
	}
	ret = append(ret, packDWord(makeTLVHeader(tlvTypeEnd, endTLVLenDwords))...)
	return ret
}

func packOpTLV(regID uint32) []byte {
	ret := make([]byte, 0, opTLVLenDwords*dwordSizeBytes)
	ret = append(ret, packDWord(makeTLVHeader(tlvTypeOp, opTLVLenDwords))...)
	ret = append(ret, packDWord(makeOpMethodAndReg(regID))...)
	ret = append(ret, packDWord(0)...)
	ret = append(ret, packDWord(0)...)
	return ret
}

func packDWord(value uint32) []byte {
	ret := make([]byte, dwordSizeBytes)
	binary.BigEndian.PutUint32(ret, value)
	return ret
}

func makeTLVHeader(tType, length uint32) uint32 {
	// Match ctypes bitfield layout
	// struct TLV { res1:16, len:11, tType:5 } packed into uint32.
	// This places tType in the highest 5 bits and len in bits [16..26].
	return ((tType & 0x1F) << 27) | ((length & 0x7FF) << 16)
}

func makeOpMethodAndReg(regID uint32) uint32 {
	return (opTLVClassReg & 0xF) |
		((opTLVMethodQuery & 0x7F) << 8) |
		((opTLVRequestBit & 0x1) << 15) |
		((regID & 0xFFFF) << 16)
}

func unpackTLV(buffer []byte) (map[string]uint64, error) {
	offset := opTLVLenDwords * dwordSizeBytes
	if len(buffer) < offset+dwordSizeBytes {
		return nil, fmt.Errorf("PRM response too short: %d", len(buffer))
	}

	regHeader := binary.BigEndian.Uint32(buffer[offset : offset+dwordSizeBytes])
	regLenDwords := (regHeader >> 16) & 0x7FF
	if regLenDwords < regTLVHeaderLenDwords {
		return nil, fmt.Errorf("invalid register TLV length: %d", regLenDwords)
	}

	offset += dwordSizeBytes
	regPayloadBytes := int(regLenDwords-regTLVHeaderLenDwords) * dwordSizeBytes
	if len(buffer) < offset+regPayloadBytes {
		return nil, fmt.Errorf("PRM register payload truncated: need %d bytes, have %d", offset+regPayloadBytes, len(buffer))
	}

	return unpackPPCNT(buffer[offset : offset+regPayloadBytes])
}

func unpackPPCNT(buffer []byte) (map[string]uint64, error) {
	if len(buffer) < 2*dwordSizeBytes {
		return nil, fmt.Errorf("PPCNT payload too short: %d", len(buffer))
	}

	group := binary.BigEndian.Uint32(buffer[0:dwordSizeBytes]) & 0x3F
	if group != ppcntGroupPLR {
		return nil, fmt.Errorf("unexpected PPCNT group 0x%x", group)
	}

	return unpackPPCNTGrpX22PLR(buffer[2*dwordSizeBytes:])
}

func unpackPPCNTGrpX22PLR(buffer []byte) (map[string]uint64, error) {
	requiredLen := len(plrCounterFields) * 2 * dwordSizeBytes
	if len(buffer) < requiredLen {
		return nil, fmt.Errorf("PLR payload too short: need %d bytes, have %d", requiredLen, len(buffer))
	}

	metrics := make(map[string]uint64, len(plrCounterFields))
	offset := 0
	for _, field := range plrCounterFields {
		high := binary.BigEndian.Uint32(buffer[offset : offset+dwordSizeBytes])
		offset += dwordSizeBytes
		low := binary.BigEndian.Uint32(buffer[offset : offset+dwordSizeBytes])
		offset += dwordSizeBytes
		metrics[field] = (uint64(high) << 32) | uint64(low)
	}

	return metrics, nil
}
