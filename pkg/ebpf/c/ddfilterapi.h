#pragma once

#ifndef _STDINT
// we're using an older SDK that doesn't have these types.  Define them for clarity
typedef unsigned long long  uint64_t;
#endif

// this type doesn't seem to be defined anyway
typedef unsigned char       uint8_t;
typedef long LONG;

// Define a version signature so that the driver won't load out of date structures, etc.
#define DD_FILTER_VERSION       0x01
#define DD_FILTER_SIGNATURE     ((uint64_t)0xDDFD << 32 | DD_FILTER_VERSION)

// For more information on defining control codes, see
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
#pragma pack(1)
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

#define    DIRECTION_INBOUND    ((uint64_t)0)
#define    DIRECTION_OUTBOUND   ((uint64_t)1)

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
    uint64_t          af;     //! address family to filter

    FILTER_ADDRESS  sourceAddress;
    FILTER_ADDRESS  destAddress;
    uint64_t        sourcePort;
    uint64_t        destinationPort;
    uint64_t        protocol;
    uint64_t        direction;
} FILTER_DEFINITION;

/*!
 * PACKET_HEADER structure
 *
 * provided by the driver during the upcall with implementation specific
 * information in the header.
 */
typedef struct filterPacketHeader
{
    uint64_t        filterVersion;
    uint64_t		sz;		                //! size of packet header, including this field
    uint64_t        skippedSinceLast;
    uint64_t        filterId;
    uint64_t		direction;              //! direction of packet
    uint64_t		pktSize;                //! size of packet
    uint64_t		af;		                //! address family of packet

    // data follows
} PACKET_HEADER, *PPACKET_HEADER;
#pragma pack()
