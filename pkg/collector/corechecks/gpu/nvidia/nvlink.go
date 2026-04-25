// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"fmt"
	"math"

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

const nvlinkFECHistoryMetricName = "nvlink.errors.fec"

var nvlinkFECHistoryFieldIDs = []uint32{
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_0,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_1,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_2,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_3,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_4,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_5,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_6,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_7,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_8,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_9,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_10,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_11,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_12,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_13,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_14,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_15,
}

type nvlinkCollector struct {
	device         ddnvml.Device
	portCollectors map[int][]portCollector
}

type portCollector func(port int) ([]Metric, error)

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

	c := &nvlinkCollector{
		device:         device,
		portCollectors: make(map[int][]portCollector, totalPorts),
	}

	for port := 1; port <= totalPorts; port++ {
		c.portCollectors[port] = []portCollector{
			c.readPLRCounters,
			c.readFECHistory,
		}
	}

	c.removeUnsupportedPorts()
	if len(c.portCollectors) == 0 {
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

	for port, collectors := range c.portCollectors {
		for _, collector := range collectors {
			metrics, err := collector(port)
			if err != nil {
				multiErr = multierror.Append(multiErr, fmt.Errorf("collect metrics for port %d using collector %T: %w", port, collector, err))
				continue
			}

			for i := range metrics {
				metrics[i].Tags = append(metrics[i].Tags, fmt.Sprintf("nvlink_port:%d", port))
			}

			allMetrics = append(allMetrics, metrics...)
		}
	}

	return allMetrics, multiErr
}

func (c *nvlinkCollector) removeUnsupportedPorts() {
	supportedPortCollectors := make(map[int][]portCollector)
	for port, collectors := range c.portCollectors {
		var supportedCollectors []portCollector
		for _, collector := range collectors {
			_, err := collector(port)
			if ddnvml.IsAPIUnsupportedOnDevice(err, c.device) {
				continue
			}
			supportedCollectors = append(supportedCollectors, collector)
		}

		if len(supportedCollectors) > 0 {
			supportedPortCollectors[port] = supportedCollectors
		}
	}

	c.portCollectors = supportedPortCollectors
}

func (c *nvlinkCollector) readFECHistory(port int) ([]Metric, error) {
	fields := make([]nvml.FieldValue, len(nvlinkFECHistoryFieldIDs))
	scopeID := uint32(port - 1)
	for i, fieldID := range nvlinkFECHistoryFieldIDs {
		fields[i] = nvml.FieldValue{
			FieldId: fieldID,
			ScopeId: scopeID,
		}
	}

	if err := c.device.GetFieldValues(fields); err != nil {
		return nil, fmt.Errorf("get FEC history field values for scope %d: %w", scopeID, err)
	}

	var fecMetrics []Metric
	var multiErr error
	for bucket, fieldValue := range fields {
		if fieldValue.NvmlReturn != uint32(nvml.SUCCESS) {
			multiErr = multierror.Append(multiErr, fmt.Errorf("field %d returned %s for scope %d", fieldValue.FieldId, nvml.ErrorString(nvml.Return(fieldValue.NvmlReturn)), scopeID))
			continue
		}

		count, err := fieldValueToNumber[uint64](nvml.ValueType(fieldValue.ValueType), fieldValue.Value)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("convert FEC history field %d for scope %d: %w", fieldValue.FieldId, scopeID, err))
			continue
		}
		if count > math.MaxInt64 {
			multiErr = multierror.Append(multiErr, fmt.Errorf("FEC history field %d for scope %d exceeds int64: %d", fieldValue.FieldId, scopeID, count))
			continue
		}

		fecMetrics = append(fecMetrics, Metric{
			Name:     nvlinkFECHistoryMetricName,
			Type:     metrics.HistogramType,
			Value:    float64(count),
			Priority: Medium,
			HistogramBucket: &Bucket{
				Bounds:          [2]float64{float64(bucket), float64(bucket + 1)},
				Monotonic:       true,
				FlushFirstValue: false,
			},
		})
	}

	if len(fecMetrics) == 0 && multiErr != nil {
		return nil, multiErr
	}

	return fecMetrics, multiErr
}

func (c *nvlinkCollector) readPLRCounters(port int) ([]Metric, error) {
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
	counters, err := unpackTLV(prm.InData[:])
	if err != nil {
		return nil, fmt.Errorf("unpack PPCNT TLV: %w", err)
	}

	var plrMetrics []Metric
	var multiErr error
	for _, field := range plrCounterFields {
		value, found := counters[field]
		if !found {
			multiErr = multierror.Append(multiErr, fmt.Errorf("missing PLR counter %q for port %d", field, port))
			continue
		}

		plrMetrics = append(plrMetrics, Metric{
			Name:  field,
			Value: float64(value),
			Type:  metrics.GaugeType,
		})
	}

	return plrMetrics, multiErr
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
