// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package driver

/*
//! These includes are needed to use constants defined in the ddnpmapi
#include <WinDef.h>
#include <WinIoCtl.h>

//! Defines the objects used to communicate with the driver as well as its control codes
#include "ddnpmapi.h"
#include <stdlib.h>
*/
import "C"

const Signature = C.DD_NPMDRIVER_SIGNATURE

const (
	GetStatsIOCTL             = C.DDNPMDRIVER_IOCTL_GETSTATS
	SetFlowFilterIOCTL        = C.DDNPMDRIVER_IOCTL_SET_FLOW_FILTER
	SetDataFilterIOCTL        = C.DDNPMDRIVER_IOCTL_SET_DATA_FILTER
	GetFlowsIOCTL             = C.DDNPMDRIVER_IOCTL_GET_FLOWS
	SetMaxOpenFlowsIOCTL      = C.DDNPMDRIVER_IOCTL_SET_MAX_OPEN_FLOWS
	SetMaxClosedFlowsIOCTL    = C.DDNPMDRIVER_IOCTL_SET_MAX_CLOSED_FLOWS
	FlushPendingHttpTxnsIOCTL = C.DDNPMDRIVER_IOCTL_FLUSH_PENDING_HTTP_TRANSACTIONS
	EnableHttpIOCTL           = C.DDNPMDRIVER_IOCTL_ENABLE_HTTP
)

type FilterAddress C.struct__filterAddress

type FilterDefinition C.struct__filterDefinition

const FilterDefinitionSize = C.sizeof_struct__filterDefinition

type FilterPacketHeader C.struct_filterPacketHeader

const FilterPacketHeaderSize = C.sizeof_struct_filterPacketHeader

type FlowStats C.struct__flow_handle_stats
type TransportStats C.struct__transport_handle_stats
type HttpStats C.struct__http_handle_stats
type Stats C.struct__stats

const StatsSize = C.sizeof_struct__stats

type PerFlowData C.struct__perFlowData
type TCPFlowData C.struct__tcpFlowData
type UDPFlowData C.struct__udpFlowData

const PerFlowDataSize = C.sizeof_struct__perFlowData

const (
	FlowDirectionMask     = C.FLOW_DIRECTION_MASK
	FlowDirectionBits     = C.FLOW_DIRECTION_BITS
	FlowDirectionInbound  = C.FLOW_DIRECTION_INBOUND
	FlowDirectionOutbound = C.FLOW_DIRECTION_OUTBOUND

	FlowClosedMask         = C.FLOW_CLOSED_MASK
	TCPFlowEstablishedMask = C.TCP_FLOW_ESTABLISHED_MASK
)

const (
	DirectionInbound  = C.DIRECTION_INBOUND
	DirectionOutbound = C.DIRECTION_OUTBOUND
)

const (
	LayerTransport = C.FILTER_LAYER_TRANSPORT
)

type HttpTransactionType C.struct__HttpTransactionType
type ConnTupleType C.struct__ConnTupleType
type HttpMethodType C.enum__HttpMethodType

const (
	HttpBatchSize           = C.HTTP_BATCH_SIZE
	HttpBufferSize          = C.HTTP_BUFFER_SIZE
	HttpTransactionTypeSize = C.sizeof_struct__HttpTransactionType
)
