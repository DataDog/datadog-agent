// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

func requestLatency(responseLastSeen uint64, requestStarted uint64) float64 {
	return protocols.NSTimestampToFloat(uint64(responseLastSeen - requestStarted))
}

func isIPV4(tup *driver.ConnTupleType) bool {
	return tup.Family == windows.AF_INET
}

//nolint:revive // TODO(WKIT) Fix revive linter
func ipLow(isIp4 bool, addr [16]uint8) uint64 {
	// Source & dest IP are given to us as a 16-byte slices in network byte order (BE). To convert to
	// low/high representation, we must convert to host byte order (LE).
	if isIp4 {
		return uint64(binary.LittleEndian.Uint32(addr[:4]))
	}
	return binary.LittleEndian.Uint64(addr[8:])
}

//nolint:revive // TODO(WKIT) Fix revive linter
func ipHigh(isIp4 bool, addr [16]uint8) uint64 {
	if isIp4 {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(addr[:8])
}

func srcIPLow(tup *driver.ConnTupleType) uint64 {
	return ipLow(isIPV4(tup), tup.LocalAddr)
}

func srcIPHigh(tup *driver.ConnTupleType) uint64 {
	return ipHigh(isIPV4(tup), tup.LocalAddr)
}

func dstIPLow(tup *driver.ConnTupleType) uint64 {
	return ipLow(isIPV4(tup), tup.RemoteAddr)
}

func dstIPHigh(tup *driver.ConnTupleType) uint64 {
	return ipHigh(isIPV4(tup), tup.RemoteAddr)
}

// --------------------------
//
// driverHttpTX interface
//

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) RequestLatency() float64 {
	return requestLatency(tx.Txn.ResponseLastSeen, tx.Txn.RequestStarted)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: srcIPHigh(&tx.Txn.Tup),
		SrcIPLow:  srcIPLow(&tx.Txn.Tup),
		DstIPHigh: dstIPHigh(&tx.Txn.Tup),
		DstIPLow:  dstIPLow(&tx.Txn.Tup),
		SrcPort:   tx.Txn.Tup.LocalPort,
		DstPort:   tx.Txn.Tup.RemotePort,
	}
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) Method() Method {
	return Method(tx.Txn.RequestMethod)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) StatusCode() uint16 {
	return tx.Txn.ResponseStatusCode
}

// Static Tags are not part of windows driver http transactions
//
//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) StaticTags() uint64 {
	return 0
}

// Dynamic Tags are not part of windows driver http transactions
//
//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) DynamicTags() []string {
	if len(tx.AppPool) != 0 || len(tx.SiteName) != 0 {
		return []string{
			fmt.Sprintf("http.iis.app_pool:%v", tx.AppPool),
			fmt.Sprintf("http.iis.site:%v", tx.SiteID),
			fmt.Sprintf("http.iis.sitename:%v", tx.SiteName),
		}
	}
	return nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) String() string {
	var output strings.Builder
	output.WriteString("httpTX{")
	output.WriteString("\n  Method: '" + tx.Method().String() + "', ")
	output.WriteString("\n  MaxRequest: '" + strconv.Itoa(int(tx.Txn.MaxRequestFragment)) + "', ")
	//output.WriteString("Fragment: '" + hex.EncodeToString(tx.RequestFragment[:]) + "', ")
	output.WriteString("\n  Fragment: '" + string(tx.RequestFragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

// Windows does not have incomplete http transactions because flows in the windows driver
// see both directions of traffic
//
//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) Incomplete() bool {
	return false
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) Path(buffer []byte) ([]byte, bool) {
	return computePath(buffer, tx.RequestFragment)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) SetStatusCode(code uint16) {
	tx.Txn.ResponseStatusCode = code
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) ResponseLastSeen() uint64 {
	return tx.Txn.ResponseLastSeen
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) SetResponseLastSeen(ls uint64) {
	tx.Txn.ResponseLastSeen = ls
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) RequestStarted() uint64 {
	return tx.Txn.RequestStarted
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tx *WinHttpTransaction) SetRequestMethod(m Method) {
	tx.Txn.RequestMethod = uint32(m)
}
