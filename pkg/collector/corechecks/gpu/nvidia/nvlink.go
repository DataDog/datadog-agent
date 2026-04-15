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
	"nvlink.plr_rcv_codes",
	"nvlink.plr_rcv_code_err",
	"nvlink.plr_rcv_uncorrectable_code",
	"nvlink.plr_xmit_codes",
	"nvlink.plr_xmit_retry_codes",
	"nvlink.plr_xmit_retry_events",
	"nvlink.plr_sync_events",
	"nvlink.plr_codes_loss",
	"nvlink.plr_xmit_retry_events_within_t_sec_max",
}

type nvlinkCollector struct {
	device ddnvml.Device
	ports  []int
}

func getNVLinkCount(device ddnvml.Device) (int, error) {
	fields := []nvml.FieldValue{{
		FieldId: nvml.FI_DEV_NVLINK_LINK_COUNT,
		ScopeId: 0,
	}}

	if err := device.GetFieldValues(fields); err != nil {
		return 0, fmt.Errorf("get NVLink link count: %w", err)
	}

	totalPorts, err := fieldValueToNumber[int](nvml.ValueType(fields[0].ValueType), fields[0].Value)
	if err != nil {
		return 0, fmt.Errorf("convert NVLink link count: %w", err)
	}
	return totalPorts, nil
}

func newNVLinkCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	totalPorts, err := getNVLinkCount(device)
	if err != nil {
		if ddnvml.IsAPIUnsupportedOnDevice(err, device) {
			return nil, errUnsupportedDevice
		}
		return nil, err
	}
	if totalPorts <= 0 {
		return nil, errUnsupportedDevice
	}

	ports := make([]int, 0, totalPorts)
	for port := 1; port <= totalPorts; port++ {
		ports = append(ports, port)
	}

	c := &nvlinkCollector{
		device: device,
		ports:  ports,
	}
	c.removeUnsupportedPorts()
	if len(c.ports) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *nvlinkCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *nvlinkCollector) Name() CollectorName {
	return "nvlink"
}

func (c *nvlinkCollector) Collect() ([]Metric, error) {
	var (
		allMetrics []Metric
		multiErr   error
	)

	for _, port := range c.ports {
		counters, err := c.readPortCounters(port)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("read PLR counters for port %d: %w", port, err))
			continue
		}

		for _, field := range plrCounterFields {
			value, found := counters[field]
			if !found {
				multiErr = multierror.Append(multiErr, fmt.Errorf("missing PLR counter %q for port %d", field, port))
				continue
			}

			allMetrics = append(allMetrics, Metric{
				Name:  field,
				Value: float64(value),
				Type:  metrics.GaugeType,
				Tags: []string{
					fmt.Sprintf("nvlink_port:%d", port),
				},
				Priority: Medium,
			})
		}
	}

	if len(allMetrics) == 0 && multiErr != nil {
		return nil, multiErr
	}

	return allMetrics, multiErr
}

func (c *nvlinkCollector) removeUnsupportedPorts() {
	ports := c.ports[:0]
	for _, port := range c.ports {
		_, err := c.readPortCounters(port)
		if err == nil {
			ports = append(ports, port)
			continue
		}
		if ddnvml.IsAPIUnsupportedOnDevice(err, c.device) {
			continue
		}
		ports = append(ports, port)
	}

	c.ports = ports
}

func (c *nvlinkCollector) readPortCounters(port int) (map[string]uint64, error) {
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
	return ((length & 0x7FF) << 5) | (tType & 0x1F)
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
	regLenDwords := (regHeader >> 5) & 0x7FF
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
