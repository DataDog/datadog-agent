//+build ignore

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
	GetStatsIOCTL      = C.DDNPMDRIVER_IOCTL_GETSTATS
	SetFlowFilterIOCTL = C.DDNPMDRIVER_IOCTL_SET_FLOW_FILTER
	SetDataFilterIOCTL = C.DDNPMDRIVER_IOCTL_SET_DATA_FILTER
	SetMaxFlowsIOCTL   = C.DDNPMDRIVER_IOCTL_SET_MAX_FLOWS
)

type FilterAddress C.struct__filterAddress

type FilterDefinition C.struct__filterDefinition

const FilterDefinitionSize = C.sizeof_struct__filterDefinition

type FilterPacketHeader C.struct_filterPacketHeader

const FilterPacketHeaderSize = C.sizeof_struct_filterPacketHeader

type HandleStats C.struct__handle_stats
type FlowStats C.struct__flow_handle_stats
type TransportStats C.struct__transport_handle_stats
type Stats C.struct__stats
type DriverStats C.struct_driver_stats

const DriverStatsSize = C.sizeof_struct_driver_stats

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
