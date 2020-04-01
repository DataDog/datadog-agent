#pragma once

#ifndef _STDINT
// we're using an older SDK that doesn't have these types.  Define them for clarity
typedef unsigned long long  uint64_t;
typedef unsigned long       uint32_t;
typedef unsigned short      uint16_t;
#endif

// for usage when building with the tracer
#ifndef _MSC_VER_
typedef long LONG;
#endif

// this type doesn't seem to be defined anyway
typedef unsigned char       uint8_t;

// define a version signature so that the driver won't load out of date structures, etc.
#define DD_FILTER_VERSION       0x03
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

#define DDFILTER_IOCTL_SET_FILTER  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x803, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDFILTER_IOCTL_SET_HANDLE_BUFFER_CONFIG CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x804, \
                                              METHOD_BUFFERED, \
                                              FILE_ANY_ACCESS)

#define DDFILTER_IOCTL_GET_HANDLE_BUFFER_CONFIG CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x805, \
                                              METHOD_BUFFERED, \
                                              FILE_ANY_ACCESS)
#pragma pack(1)

/**
 * per-handle buffer config
 *
 * used to retrieve and configure the per-handle packet buffers
 *
 * number is the number of packets that can be buffered before packets
 * will be dropped or pended (depending on the filter type)
 *
 * size is the maximum number of bytes copied per-packet
 */

#define PACKET_MODE_NONE                        0x0 // no options set

// HEADER_ONLY mode will only copy IP & Transport header to buffer
#define PACKET_REPORT_MODE_HEADER_ONLY          0x10

// HEADER_AND_DATA will copy IP & transport header, plus data up
// to the size of the buffer specified in sizeOfBuffer
#define PACKET_REPORT_MODE_HEADER_AND_DATA      0x20

#define IS_MODE_HEADER_ONLY(x)      (((x) & PACKET_REPORT_MODE_HEADER_ONLY) == PACKET_REPORT_MODE_HEADER_ONLY)
#define IS_MODE_HEADER_AND_DATA(x)  (((x) & PACKET_REPORT_MODE_HEADER_ONLY) == PACKET_REPORT_MODE_HEADER_AND_DATA)
typedef struct _handle_buffer_cfg
{
    uint64_t        filterVersion;
    uint32_t        numPacketBuffers;
    uint32_t        sizeOfBuffer;
    uint64_t        packetMode;
} HANDLE_BUFFER_CONFIG;
/*!
 STATS structure.

 This structure is used to collect various types of statistics from the driver
 */
typedef struct _stats
{
    volatile LONG		read_calls;		//! number of read calls to the driver
    volatile LONG       read_bytes;

    volatile LONG       read_calls_outstanding;
    volatile LONG       read_calls_completed;

    volatile LONG       read_calls_cancelled;
    volatile LONG       read_packets_skipped;

    volatile LONG		write_calls;	//! number of write calls to the driver
    volatile LONG       write_bytes;

    volatile LONG		ioctl_calls;	//! number of ioctl calls to the driver
    volatile LONG       padding;        // only necessary with an odd number of stats.
    
    volatile LONG       packets_processed; // number of packets through the driver
    volatile LONG       packets_queued;     // number of packets transferred to the internal queue
    volatile LONG       packets_reported; // number of packets sent up
    volatile LONG       packets_pended;     // packets that had to be queued rather than send directly
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

#define     FILTER_LAYER_ALE_CONNECT    ((uint64_t) 2)
#define     FILTER_LAYER_ALE_RECVCONN   ((uint64_t) 3)
#define     FILTER_LAYER_ALE_CLOSURE    ((uint64_t) 4)
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

    FILTER_ADDRESS  sourceAddress;
    FILTER_ADDRESS  destAddress;
    uint64_t        sourcePort;
    uint64_t        destinationPort;
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
typedef struct filterBufferHeader
{
    uint64_t        filterVersion;
    uint64_t        sz;		                //! size of packet header, including this field
    uint64_t        skippedSinceLast;
    uint64_t        filterId;
    uint64_t        pktSize;                //! size of packet
    uint64_t        numPackets;             //! number of packets in buffer

    // data follows
} BUFFER_HEADER, *PBUFFER_HEADER;

#define PACKET_TYPE_MASK            ((uint16_t) 0x03)
#define PACKET_TYPE_DATA            ((uint16_t) 0x00)
#define PACKET_TYPE_NEW_FLOW        ((uint16_t) 0x01)
#define PACKET_TYPE_END_FLOW        ((uint16_t) 0x02)

#define PACKET_DIRECTION_MASK       ((uint16_t) 0x10)
#define PACKET_DIRECTION_INBOUND    ((uint16_t) 0x00)
#define PACKET_DIRECTION_OUTBOUND   ((uint16_t) 0x10)

#define IS_PACKET_TYPE_DATA(x)      (((x)->info & PACKET_TYPE_MASK) == PACKET_TYPE_DATA)
#define IS_PACKET_TYPE_NEW_FLOW(x)  (((x)->info & PACKET_TYPE_MASK) == PACKET_TYPE_NEW_FLOW)
#define IS_PACKET_TYPE_END_FLOW(x)  (((x)->info & PACKET_TYPE_MASK) == PACKET_TYPE_END_FLOW)

#define SET_PACKET_TYPE_DATA(x)     (x)->info |= (PACKET_TYPE_MASK & PACKET_TYPE_DATA)
#define SET_PACKET_TYPE_NEW_FLOW(x) (x)->info |= (PACKET_TYPE_MASK & PACKET_TYPE_NEW_FLOW)
#define SET_PACKET_TYPE_END_FLOW(x) (x)->info |= (PACKET_TYPE_MASK & PACKET_TYPE_END_FLOW)

#define IS_PACKET_DIRECTION_INBOUND(x)  (((x)->info & PACKET_DIRECTION_MASK) == PACKET_DIRECTION_INBOUND)
#define IS_PACKET_DIRECTION_OUTBOUND(x) (((x)->info & PACKET_DIRECTION_MASK) == PACKET_DIRECTION_OUTBOUND)

#define SET_PACKET_DIRECTION_INBOUND(x)   (x)->info |= (PACKET_DIRECTION_MASK & PACKET_DIRECTION_INBOUND)
#define SET_PACKET_DIRECTION_OUTBOUND(x)  (x)->info |= (PACKET_DIRECTION_MASK & PACKET_DIRECTION_OUTBOUND)
typedef struct packetHeader
{
    uint16_t            size;                   // size of packet (amount of data including this header)
    uint16_t            info;                   // bitmask indicating what type of packet
    uint16_t            reserved1;
    uint16_t            reserved2;
    uint64_t            ownerPid;               // if known, (-1) if unknown
    uint64_t            flowId;
} PACKET_HEADER, *PPACKET_HEADER;

// appears immediately after packetHeader if this is a notification of
// the start or end of a connection, rather than an actual data packet

typedef struct flowNotification
{
    // current flag bits
    // lowest bit indicates address family
    // lowest 8 bits of 2nd word is the protocol
    // highest 16 bits is the local port
    // next highest 16 bits is the remoteport

    // flags is 0x lplp rprp 0000 PP0A
    // where PP are the protocol bits, and the lowest bit of A is the address family
    uint64_t            protoflags;
    uint8_t             localAddress[16];
    uint8_t             remoteAddress[16];
} FLOW_NOTIFICATION, *PFLOW_NOTIFICATION;

#define FLOW_NOTIFICATION_AF_MASK   0x01
#define FLOW_NOTIFICATION_AF_INET   0x00
#define FLOW_NOTIFICATION_AF_INET6  0x01

#define IS_FLOW_AF_INET(x)  (((x)->protoflags & FLOW_NOTIFICATION_AF_MASK) == FLOW_NOTIFICATION_AF_INET)
#define IS_FLOW_AF_INET6(x) (((x)->protoflags & FLOW_NOTIFICATION_AF_MASK) == FLOW_NOTIFICATION_AF_INET6)

#define SET_FLOW_NOTIFICATION_AF_INET(x)    (x)->protoflags |= (FLOW_NOTIFICATION_AF_MASK & FLOW_NOTIFICATION_AF_INET)
#define SET_FLOW_NOTIFICATION_AF_INET6(x)   (x)->protoflags |= (FLOW_NOTIFICATION_AF_MASK & FLOW_NOTIFICATION_AF_INET6)

#define FLOW_NOTIFICATION_PROTOCOL_MASK 0xFF00
#define FLOW_NOTIFICATION_PROTOCOL_BITS 8

#define GET_FLOW_NOTIFICATION_PROTOCOL(x)  (((x)->protoflags & FLOW_NOTIFICATION_PROTOCOL_MASK) >> FLOW_NOTIFICATION_PROTOCOL_BITS)
#define SET_FLOW_NOTIFICATION_PROTOCOL(x, y) \
    (x)->protoflags |= (((y) << FLOW_NOTIFICATION_PROTOCOL_BITS) & FLOW_NOTIFICATION_PROTOCOL_MASK)

#define FLOW_NOTIFICATION_LOCAL_PORT_SHIFT_BITS 48
#define FLOW_NOTIFICATION_REMOTE_PORT_SHIFT_BITS 32
#define FLOW_NOTIFICATION_LOCAL_PORT_MASK   0xFFFF000000000000
#define FLOW_NOTIFICATION_REMOTE_PORT_MASK  0x0000FFFF00000000

#define GET_FLOW_NOTIFICATION_LOCAL_PORT(x) \
    ((uint16_t)(((x)->protoflags & FLOW_NOTIFICATION_LOCAL_PORT_MASK) >> FLOW_NOTIFICATION_LOCAL_PORT_SHIFT_BITS) & 0xFFFF)
#define SET_FLOW_NOTIFICATION_LOCAL_PORT(x, p) \
    (x)->protoflags |= ((((uint64_t)p) << FLOW_NOTIFICATION_LOCAL_PORT_SHIFT_BITS) & FLOW_NOTIFICATION_LOCAL_PORT_MASK)

#define GET_FLOW_NOTIFICATION_REMOTE_PORT(x) \
    ((uint16_t)(((x)->protoflags & FLOW_NOTIFICATION_REMOTE_PORT_MASK) >> FLOW_NOTIFICATION_REMOTE_PORT_SHIFT_BITS) & 0xFFFF)
#define SET_FLOW_NOTIFICATION_REMOTE_PORT(x, p) \
    (x)->protoflags |= ((((uint64_t)p) << FLOW_NOTIFICATION_REMOTE_PORT_SHIFT_BITS) & FLOW_NOTIFICATION_REMOTE_PORT_MASK)

#pragma pack()
