// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

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
	EnableClassifyIOCTL       = C.DDNPMDRIVER_IOCTL_SET_CLASSIFY
	SetClosedFlowsLimitIOCTL  = C.DDNPMDRIVER_IOCTL_SET_CLOSED_FLOWS_NOTIFY
	GetOpenFlowsIOCTL         = C.DDNPMDRIVER_IOCTL_GET_OPEN_FLOWS
	GetClosedFlowsIOCTL       = C.DDNPMDRIVER_IOCTL_GET_CLOSED_FLOWS
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

type PerFlowData C.struct__userFlowData
type TCPFlowData C.struct__tcpFlowData
type UDPFlowData C.struct__udpFlowData

const PerFlowDataSize = C.sizeof_struct__userFlowData

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
type HttpConfigurationSettings C.struct__HttpConfigurationSettings
type ConnTupleType C.struct__ConnTupleType
type HttpMethodType C.enum__HttpMethodType
type ClassificationSettings C.struct__ClassificationConfigurationSettings

type TcpConnectionStatus C.enum__ConnectionStatus

const (
	TcpStatusEstablished = C.CONN_STAT_ESTABLISHED
)
const (
	HttpTransactionTypeSize        = C.sizeof_struct__HttpTransactionType
	HttpSettingsTypeSize           = C.sizeof_struct__HttpConfigurationSettings
	ClassificationSettingsTypeSize = C.sizeof_struct__ClassificationConfigurationSettings
)

const (
	ClassificationUnclassified           = C.CLASSIFICATION_UNCLASSIFIED
	ClassificationClassified             = C.CLASSIFICATION_CLASSIFIED
	ClassificationUnableInsufficientData = C.CLASSIFICATION_UNABLE_INSUFFICIENT_DATA
	ClassificationUnknown                = C.CLASSIFICATION_UNKNOWN

	ClassificationRequestUnclassified = C.CLASSIFICATION_REQUEST_UNCLASSIFIED
	ClassificationRequestHTTPUnknown  = C.CLASSIFICATION_REQUEST_HTTP_UNKNOWN
	ClassificationRequestHTTPPost     = C.CLASSIFICATION_REQUEST_HTTP_POST
	ClassificationRequestHTTPPut      = C.CLASSIFICATION_REQUEST_HTTP_PUT
	ClassificationRequestHTTPPatch    = C.CLASSIFICATION_REQUEST_HTTP_PATCH
	ClassificationRequestHTTPGet      = C.CLASSIFICATION_REQUEST_HTTP_GET
	ClassificationRequestHTTPHead     = C.CLASSIFICATION_REQUEST_HTTP_HEAD
	ClassificationRequestHTTPOptions  = C.CLASSIFICATION_REQUEST_HTTP_OPTIONS
	ClassificationRequestHTTPDelete   = C.CLASSIFICATION_REQUEST_HTTP_DELETE
	ClassificationRequestHTTPLast     = C.CLASSIFICATION_REQUEST_HTTP_LAST

	ClassificationRequestHTTP2 = C.CLASSIFICATION_REQUEST_HTTP2

	ClassificationRequestTLS  = C.CLASSIFICATION_REQUEST_TLS
	ClassificationResponseTLS = C.CLASSIFICATION_RESPONSE_TLS

	ALPNProtocolHTTP2  = C.ALPN_PROTOCOL_HTTP2
	ALPNProtocolHTTP11 = C.ALPN_PROTOCOL_HTTP11

	ClassificationResponseUnclassified = C.CLASSIFICATION_RESPONSE_UNCLASSIFIED
	ClassificationResponseHTTP         = C.CLASSIFICATION_RESPONSE_HTTP
)
