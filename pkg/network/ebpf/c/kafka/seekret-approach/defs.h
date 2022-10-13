#pragma once

#define BPF_MAP_SEEKRET(_name, _type, _key_type, _value_type, _max_entries) \
struct bpf_map_def SEC("maps") _name = { \
  .type = _type, \
  .key_size = sizeof(_key_type), \
  .value_size = sizeof(_value_type), \
  .max_entries = _max_entries, \
};

#define BPF_HASH(_name, _key_type, _value_type) \
BPF_MAP_SEEKRET(_name, BPF_MAP_TYPE_HASH, _key_type, _value_type, 102400)

#define BPF_ARRAY(_name, _value_type, _max_entries) \
BPF_MAP_SEEKRET(_name, BPF_MAP_TYPE_ARRAY, u32, _value_type, _max_entries)

#define BPF_PERCPU_ARRAY(_name, _value_type, _max_entries) \
BPF_MAP_SEEKRET(_name, BPF_MAP_TYPE_PERCPU_ARRAY, u32, _value_type, _max_entries)

#define BPF_PERF_OUTPUT(_name) \
BPF_MAP_SEEKRET(_name, BPF_MAP_TYPE_PERF_EVENT_ARRAY, int, __u32, 1024)

#define MAX_PAYLOAD_SIZE_BYTES 40960 // 40KB
#define MAX_EVENT_DATA_SIZE 30720
#define MAX_ITERATIONS_FOR_DATA_EVENT 2 // 40KB / 30720
//
//#define EINPROGRESS 115
//// TODO: Get correct values
//#define NSEC_PER_SEC 1000000000L
//#define USER_HZ 100

///* Supported address families. */
//#define AF_UNSPEC      0
//#define AF_UNIX        1          /* Unix domain sockets */
//#define AF_LOCAL       1          /* POSIX name for AF_UNIX */
//#define AF_INET        2          /* Internet IP Protocol */
//#define AF_AX25        3          /* Amateur Radio AX.25 */
//#define AF_IPX         4          /* Novell IPX */
//#define AF_APPLETALK   5          /* AppleTalk DDP */
//#define AF_NETROM      6          /* Amateur Radio NET/ROM */
//#define AF_BRIDGE      7          /* Multiprotocol bridge */
//#define AF_ATMPVC      8          /* ATM PVCs */
//#define AF_X25         9          /* Reserved for X.25 project */
//#define AF_INET6       10         /* IP version 6 */
//#define AF_ROSE        11         /* Amateur Radio X.25 PLP */
//#define AF_DECnet      12         /* Reserved for DECnet project */
//#define AF_NETBEUI     13         /* Reserved for 802.2LLC project */
//#define AF_SECURITY    14         /* Security callback pseudo AF */
//#define AF_KEY         15         /* PF_KEY key management API */
//#define AF_NETLINK     16
//#define AF_ROUTE       AF_NETLINK /* Alias to emulate 4.4BSD */
//#define AF_PACKET      17         /* Packet family */
//#define AF_ASH         18         /* Ash */
//#define AF_ECONET      19         /* Acorn Econet */
//#define AF_ATMSVC      20         /* ATM SVCs */
//#define AF_RDS         21         /* RDS sockets */
//#define AF_SNA         22         /* Linux SNA Project (nutters!) */
//#define AF_IRDA        23         /* IRDA sockets */
//#define AF_PPPOX       24         /* PPPoX sockets */
//#define AF_WANPIPE     25         /* Wanpipe API Sockets */
//#define AF_LLC         26         /* Linux LLC */
//#define AF_IB          27         /* Native InfiniBand address */
//#define AF_MPLS        28         /* MPLS */
//#define AF_CAN         29         /* Controller Area Network */
//#define AF_TIPC        30         /* TIPC sockets */
//#define AF_BLUETOOTH   31         /* Bluetooth sockets */
//#define AF_IUCV        32         /* IUCV sockets */
//#define AF_RXRPC       33         /* RxRPC sockets */
//#define AF_ISDN        34         /* mISDN sockets */
//#define AF_PHONET      35         /* Phonet sockets */
//#define AF_IEEE802154  36         /* IEEE802154 sockets */
//#define AF_CAIF        37         /* CAIF sockets */
//#define AF_ALG         38         /* Algorithm sockets */
//#define AF_NFC         39         /* NFC sockets */
//#define AF_VSOCK       40         /* vSockets */
//#define AF_KCM         41         /* Kernel Connection Multiplexor */
//#define AF_QIPCRTR     42         /* Qualcomm IPC Router */
//#define AF_SMC         43         /* smc sockets: reserve number for PF_SMC protocol family that reuses AF_INET address family */
//
//#define AF_MAX         45         /* For now.. */

struct trace_entry {
	short unsigned int type;
	unsigned char flags;
	unsigned char preempt_count;
	int pid;
};

struct trace_event_raw_sys_enter {
	struct trace_entry ent;
	long int id;
	long unsigned int args[6];
	char __data[0];
};

struct trace_event_raw_sys_exit {
	struct trace_entry ent;
	long int id;
	long int ret;
	char __data[0];
};
