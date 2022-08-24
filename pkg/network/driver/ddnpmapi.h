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
#define DD_NPMDRIVER_VERSION       0x12
#define DD_NPMDRIVER_SIGNATURE     ((uint64_t)0xDDFD << 32 | DD_NPMDRIVER_VERSION)

// for more information on defining control codes, see
// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/defining-i-o-control-codes
//
// Vendor codes start with 0x800

#define DDNPMDRIVER_IOCTL_GETSTATS         CTL_CODE(FILE_DEVICE_NETWORK, \
                                                0x801,                \
                                                METHOD_BUFFERED,      \
                                                FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SIMULATE_COMPLETE_READ CTL_CODE(FILE_DEVICE_NETWORK, \
                                                        0x802,              \
                                                        METHOD_BUFFERED,    \
                                                        FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SET_DATA_FILTER  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x803, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

////// DEPRECATED
#define DDNPMDRIVER_IOCTL_SET_FLOW_FILTER  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x804, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_GET_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x805, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

///// DEPRECATED
#define DDNPMDRIVER_IOCTL_SET_MAX_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x806, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SET_HTTP_FILTER CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x807, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_FLUSH_PENDING_HTTP_TRANSACTIONS CTL_CODE(FILE_DEVICE_NETWORK, \
                                                            0x808, \
                                                            METHOD_BUFFERED,\
                                                            FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SET_MAX_OPEN_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x809, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SET_MAX_CLOSED_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x80A, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SET_MAX_HTTP_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x80B, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)


#define DDNPMDRIVER_IOCTL_ENABLE_HTTP  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x80C, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_GET_OPEN_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x80D, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_GET_CLOSED_FLOWS  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x80E, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)

#define DDNPMDRIVER_IOCTL_SET_CLOSED_FLOWS_NOTIFY  CTL_CODE(FILE_DEVICE_NETWORK, \
                                              0x80F, \
                                              METHOD_BUFFERED,\
                                              FILE_ANY_ACCESS)


#pragma pack(1)

/*!
 STATS structure.

 This structure is used to collect various types of statistics from the driver
 */
typedef struct _flow_handle_stats {
    volatile LONG64         num_flow_collisions;

    volatile LONG64         num_flow_alloc_skipped_max_open_exceeded;
    volatile LONG64         num_flow_closed_dropped_max_exceeded;

    // these are kept in the flow_table structure itself,
    // and copied into the stats struct when the struct is queried.
    volatile LONG64         num_flow_structures;      // total number of open flow structures
    volatile LONG64         peak_num_flow_structures; // high water mark of numFlowStructures

    volatile LONG64         num_flow_closed_structures;  //
    volatile LONG64         peak_num_flow_closed_structures;

    volatile LONG64         open_table_adds;
    volatile LONG64         open_table_removes;
    volatile LONG64         closed_table_adds;
    volatile LONG64         closed_table_removes;

    // same for no_handle flows
    volatile LONG64         num_flows_no_handle;
    volatile LONG64         peak_num_flows_no_handle;
    volatile LONG64         num_flows_missed_max_no_handle_exceeded;

    volatile LONG64         num_packets_after_flow_closed;

} FLOW_STATS;

typedef struct _transport_handle_stats {

    volatile LONG64       read_packets_skipped;

    volatile LONG64       read_calls_requested;
    volatile LONG64       read_calls_completed;
    volatile LONG64       read_calls_cancelled;

} TRANSPORT_STATS;

typedef struct _http_handle_stats {

    volatile LONG64       txns_captured;
    volatile LONG64       txns_skipped_max_exceeded;
    volatile LONG64       ndis_buffer_non_contiguous;

} HTTP_STATS;

typedef struct _stats
{
    FLOW_STATS              flow_stats;
    TRANSPORT_STATS         transport_stats;
    HTTP_STATS              http_stats;
} STATS;

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

//#define     FILTER_LAYER_IPPACKET       ((uint64_t) 0)
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
    uint64_t        af;     //! address family to filter

    FILTER_ADDRESS  localAddress;
    FILTER_ADDRESS  remoteAddress;
    uint64_t        localPort;
    uint64_t        remotePort;
    uint64_t        protocol;
    uint64_t        direction;
    uint64_t        interfaceIndex;
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

    uint64_t        iRTT;           // initial RTT
    uint64_t        sRTT;           // smoothed RTT
    uint64_t        rttVariance;
    uint64_t        retransmitCount;
} TCP_FLOW_DATA;
typedef struct _perFlowData {
    uint64_t          flowHandle;
    uint64_t          processId;
    uint16_t          addressFamily; // AF_INET or AF_INET6
    uint16_t          protocol;
    // first byte indicates if flow has been closed
    // second byte indicates flow direction
    // flags is 0x00000DCC   (where D is direction and C is closed state)
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

    uint64_t timestamp;             // last activity on this flow.  ns since system boot

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
#define TCP_FLOW_ESTABLISHED_MASK 0x20

#define IS_FLOW_CLOSED(f) ( (((f)->flags) & FLOW_CLOSED_MASK) == FLOW_CLOSED_MASK )
#define IS_TCP_FLOW_ESTABLISHED(f) ( (((f)->flags) & TCP_FLOW_ESTABLISHED_MASK) == TCP_FLOW_ESTABLISHED_MASK )

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
    uint64_t        timestamp;              // timestamp in ns since unix epoch

    // data follows
} PACKET_HEADER, * PPACKET_HEADER;

#define USERLAND_CLOSED_FLOWS_EVENT_NAME L"\\BaseNamedObjects\\DDNPMClosedFlowsReadyEvent"
// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP_BUFFER_SIZE 25

// This controls the number of HTTP transactions read from userspace at a time
#define HTTP_BATCH_SIZE 15

#define HTTPS_PORT 443

typedef enum _HttpPacketType {
    HTTP_PACKET_UNKNOWN = 0,
    HTTP_REQUEST,
    HTTP_RESPONSE
} HTTP_PACKET_TYPE;

typedef enum _HttpMethodType {
    HTTP_METHOD_UNKNOWN = 0,
    HTTP_GET,
    HTTP_POST,
    HTTP_PUT,
    HTTP_DELETE,
    HTTP_HEAD,
    HTTP_OPTIONS,
    HTTP_PATCH
} HTTP_METHOD_TYPE;

#pragma pack(1)

typedef struct _ConnTupleType {
    uint8_t  cliAddr[16]; // only first 4 bytes valid for AF_INET, in network byte order
    uint8_t  srvAddr[16]; // ditto
    uint16_t cliPort;     // host byte order
    uint16_t srvPort;     // host byte order
    uint16_t family;      // AF_INET or AF_INET6
    uint16_t pad;         // make struct 64 bit aligned
} CONN_TUPLE_TYPE, * PCONN_TUPLE_TYPE;


typedef struct _HttpTransactionType {
    uint64_t         requestStarted;      // in ns
    uint64_t         responseLastSeen;    // in ns
    CONN_TUPLE_TYPE  tup;
    HTTP_METHOD_TYPE requestMethod;
    uint16_t         responseStatusCode;
    uint16_t         maxRequestFragment;
    uint16_t         szRequestFragment;
    uint8_t          pad[6];                  // make struct 64 bit byte aligned
    unsigned char* requestFragment;

} HTTP_TRANSACTION_TYPE, * PHTTP_TRANSACTION_TYPE;

#define USERLAND_HTTP_EVENT_NAME L"\\BaseNamedObjects\\DDNPMHttpTxnReadyEvent"
typedef struct _HttpConfigurationSettings {
    uint64_t    maxTransactions;        // max list of transactions we'll keep
    uint64_t    notificationThreshhold; // when to signal to retrieve transactions
    uint16_t    maxRequestFragment;     // max length of request fragment
} HTTP_CONFIGURATION_SETTINGS;
#pragma pack()
