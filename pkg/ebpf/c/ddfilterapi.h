#pragma once

#ifndef _STDINT
// we're using an older SDK that doesn't have these types.  Define them for clarity
typedef unsigned long long  uint64_t;
typedef unsigned long       uint32_t;
typedef unsigned short      uint16_t;
#endif

// for usage when building with the tracer
#ifndef _MSC_VER_
typedef __int64 LONG64;
#endif

// this type doesn't seem to be defined anyway
typedef unsigned char       uint8_t;

// define a version signature so that the driver won't load out of date structures, etc.
#define DD_FILTER_VERSION       0x04
#define DD_FILTER_SIGNATURE     ((uint64_t)0xDDFD << 32 | DD_FILTER_VERSION)

// for more information on defining control codes, see
// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/defining-i-o-control-codes
//
// Vendor codes start with 0x800

#define DDFILTER_IOCTL_GETSTATS         CTL_CODE(FILE_DEVICE_NETWORK, \
                                                0x801,                \
                                                METHOD_BUFFERED,      \
                                                FILE_ANY_ACCESS)

#define DDFILTER_IOCTL_SIMULATE_COMPLETE_READ CTL_CODE(FILE_DEVICE_NETWORK, \
                                                        0x802,              \
                                                        METHOD_BUFFERED,    \
                                                        FILE_ANY_ACCESS)

#define DDFILTER_IOCTL_SET_DATA_FILTER  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x803, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDFILTER_IOCTL_SET_FLOW_FILTER  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x804, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDFILTER_IOCTL_GET_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x805, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#pragma pack(1)

/*!
 STATS structure.

 This structure is used to collect various types of statistics from the driver
 */
typedef struct _handle_stats {
    volatile LONG64		read_calls;		//! number of read calls to the driver

    volatile LONG64       read_calls_outstanding;
    volatile LONG64       read_calls_completed;
    volatile LONG64       read_calls_cancelled;

    volatile LONG64       write_calls;	//! number of write calls to the driver
    volatile LONG64       write_bytes;

    volatile LONG64		  ioctl_calls;	//! number of ioctl calls to the driver

} HANDLE_STATS;

typedef struct _flow_handle_stats {
    volatile LONG64         packets_observed;  // number of packets through the driver
    volatile LONG64         packets_processed;  // number of packets actually processed
    volatile LONG64         open_flows;         // number of currently open flows
    volatile LONG64         total_flows;        // total flows processed (open + closed)

    volatile LONG64         num_flow_searches;  // number of times had to search for flow after add
    volatile LONG64         num_flow_search_misses; // number of times we missed a flow even after searching the list

    volatile LONG64         num_flow_collisions;
} FLOW_STATS;

typedef struct _transport_handle_stats {

    volatile LONG64       packets_processed; // number of packets through the driver
    volatile LONG64       read_packets_skipped;
    volatile LONG64       packets_reported; // number of packets sent up

} TRANSPORT_STATS;
typedef struct _stats
{
    HANDLE_STATS            handle_stats;
    FLOW_STATS              flow_stats;
    TRANSPORT_STATS         transport_stats;
} STATS;

/*!
 * DRIVER_STATS structure
 *
 * This structure is a rollup of the available stats.  It includes the
 * per-handle stats, and the total driver stats
 */
typedef struct driver_stats
{
    uint64_t                filterVersion;
    STATS		total;		//! stats since the driver was started
    STATS		handle;		//! stats for the file handle in question
} DRIVER_STATS;


///////////////////////////////////
// filter definitions.
//
typedef struct _filterAddress
{
    // _filterAddress defines an address to be matched, if supplied.
    // it can be ipv4 or ipv6 but not both.
    // supplying 0 for the address family means _any_ address (v4 or v6)
    uint64_t                  af; //! AF_INET, AF_INET6 or 0
    uint8_t                   v4_address[4];    // address in network byte order, so v4_address[0] = top network tuple
    uint8_t                   v4_padding[4];    // pad out to 64 bit boundary
    uint8_t                   v6_address[16];
    uint64_t                  mask; // number of mask bits.
} FILTER_ADDRESS;

#define     DIRECTION_INBOUND    ((uint64_t)0)
#define     DIRECTION_OUTBOUND   ((uint64_t)1)

#define     FILTER_LAYER_IPPACKET       ((uint64_t) 0)
#define     FILTER_LAYER_TRANSPORT      ((uint64_t) 1)

typedef struct _filterDefinition
{
    uint64_t filterVersion;
    uint64_t size;         //! size of this structure

    /**
     if supplied, the source and destination address must have the same
     address family.

     if both source and destination are applied, then the match for this filter
     is a logical AND, i.e. the source and destination both match.
     */
    uint64_t        filterLayer; //! which filter layer to apply
    uint64_t          af;     //! address family to filter

    FILTER_ADDRESS  localAddress;
    FILTER_ADDRESS  remoteAddress;
    uint64_t        localPort;
    uint64_t        remotePort;
    uint64_t        protocol;
    uint64_t        direction;

    uint64_t        v4InterfaceIndex;
    uint64_t        v6InterfaceIndex;
} FILTER_DEFINITION;

/*!
 * PACKET_HEADER structure
 *
 * provided by the driver during the upcall with implementation specific
 * information in the header.
 */

typedef struct _udpFlowData {
    uint64_t        reserved;
} UDP_FLOW_DATA;
typedef struct _tcpFlowData {
    uint32_t        startingSeq;
    uint32_t        lastSeqSent;
    uint32_t        lastSeqAcked;   // last ack we got back from other side

    uint32_t        startingAck;
    uint32_t        lastSeqReceived;
    uint32_t        lastAck;        // last ack we sent

    uint64_t        retransmitCount;
} TCP_FLOW_DATA;
typedef struct _perFlowData {
    uint64_t          flowHandle;
    uint64_t          processId;
    uint16_t          addressFamily; // AF_INET or AF_INET6
    uint16_t          protocol;
    uint32_t          flags; // for now
    uint8_t           localAddress[16];  // only first 4 bytes valid for AF_INET, in network byte order
    uint8_t           remoteAddress[16]; // ditto

    // stats common to all

    uint64_t packetsOut;
    uint64_t monotonicSentBytes;              // total bytes including ip header
    uint64_t transportBytesOut;     // payload (not including ip or transport header)

    uint64_t packetsIn;
    uint64_t monotonicRecvBytes;
    uint64_t transportBytesIn;

    uint16_t        localPort;      // host byte order
    uint16_t        remotePort;     // host byte order
    // stats unique to a particular transport
    union {
        TCP_FLOW_DATA     tcp;
        UDP_FLOW_DATA     udp;
    } protocol_u;
} PER_FLOW_DATA;

#define FLOW_DIRECTION_UNKNOWN  0x00
#define FLOW_DIRECTION_INBOUND  0x01
#define FLOW_DIRECTION_OUTBOUND 0x02
#define FLOW_DIRECTION_MASK     0x300
#define FLOW_DIRECTION_BITS     8

#define SET_FLOW_DIRECTION(f, d)         { (f)->flags |= (((d) << FLOW_DIRECTION_BITS) & FLOW_DIRECTION_MASK) ;}
#define IS_FLOW_DIRECTION_UNKNOWN(f)     ( (((f)->flags & FLOW_DIRECTION_MASK) >> FLOW_DIRECTION_BITS) == FLOW_DIRECTION_UNKNOWN)
#define IS_FLOW_DIRECTION_INBOUND(f)     ( (((f)->flags & FLOW_DIRECTION_MASK) >> FLOW_DIRECTION_BITS) == FLOW_DIRECTION_INBOUND)
#define IS_FLOW_DIRECTION_OUTBOUND(f)    ( (((f)->flags & FLOW_DIRECTION_MASK) >> FLOW_DIRECTION_BITS) == FLOW_DIRECTION_OUTBOUND)

#define FLOW_CLOSED_MASK 0x10

#define IS_FLOW_CLOSED(f) ( (((f)->flags) & FLOW_CLOSED_MASK) == FLOW_CLOSED_MASK )

/*!
 * PACKET_HEADER structure
 *
 * provided by the driver during the upcall with implementation specific
 * information in the header.
 */
typedef struct filterPacketHeader
{
    uint64_t        filterVersion;
    uint64_t        sz;		                //! size of packet header, including this field
    uint64_t        skippedSinceLast;
    uint64_t        filterId;
    uint64_t        direction;              //! direction of packet
    uint64_t        pktSize;                //! size of packet
    uint64_t        af;		                //! address family of packet
    uint64_t        ownerPid;               //! (-1) if not available

    // data follows
} PACKET_HEADER, *PPACKET_HEADER;
#pragma pack()
