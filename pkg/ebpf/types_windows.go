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
