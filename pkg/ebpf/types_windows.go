// +build windows

package ebpf

import "syscall"

const (
	FILTER_DIRECTION_INBOUND  = uint32(0)
	FILTER_DIRECTION_OUTBOUND = uint32(1)
)

type FilterAddress struct {
	addressFamily  uint16
	v4_address     [4]byte
	v6_address_pad [12]byte
	mask           uint16
}

type FilterDefinition struct {
	bufferSize    uint32
	addressFamily uint16
	srcAddr       FilterAddress
	dstAddr       FilterAddress
	sPort         uint16
	dPort         uint16
	protocol      uint16
	direction     uint32
}

type FilterPacketHeader struct {
	/*
		C struct:
		{
		    ULONG		sz;		                //! size of packet header, including this field
		    ULONG       skippedSinceLast;
		    UINT64          filterId;
		    ULONG		direction;              //! direction of packet
		    unsigned char*		pkt;	        //! pointer to packet itself (L3 header)
		    size_t		pktSize;                //! size of packet

		    unsigned short		af;		        //! address family of packet

		    // data follows
		}
	*/
	rawData [38]byte
}

type ReadBuffer struct {
	ol   syscall.Overlapped
	data [128]byte
}

/*

typedef struct _filterAddress
{
// _filterAddress defines an address to be matched, if supplied.
// it can be ipv4 or ipv6 but not both.
// supplying 0 for the address family means _any_ address (v4 or v6)
USHORT                  af; //! AF_INET, AF_INET6 or 0
union
{
struct in6_addr         v6_address;
struct in_addr          v4_address;
}u;
USHORT                  mask; // number of mask bits.
} FILTER_ADDRESS;

// ConnectionDirection struct defined in event_common.go

typedef enum _filterDirection
{
DIRECTION_INBOUND = 0,
DIRECTION_OUTBOUND = 1
} FILTER_DIRECTION;

typedef struct _filterDefinition
{
ULONG size;         //! size of this structure

/**
  if supplied, the source and destination address must have the same
  address family.
  if both source and destination are applied, then the match for this filter
  is a logical AND, i.e. the source and destination both match.

USHORT          af;     //! address family to filter

FILTER_ADDRESS  sourceAddress;
FILTER_ADDRESS  destAddress;
USHORT          sourcePort;
USHORT          destinationPort;
USHORT          protocol;
FILTER_DIRECTION    direction;
} FILTER_DEFINITION;

/*!
 * PACKET_HEADER structure
 *
 * provided by the driver during the upcall with implementation specific
 * information in the header.

typedef struct filterPacketHeader
{
ULONG		sz;		                //! size of packet header, including this field
ULONG       skippedSinceLast;
UINT64          filterId;
ULONG		direction;              //! direction of packet
unsigned char*		pkt;	        //! pointer to packet itself (L3 header)
size_t		pktSize;                //! size of packet

unsigned short		af;		        //! address family of packet

// data follows
} PACKET_HEADER, *PPACKET_HEADER;
*/
