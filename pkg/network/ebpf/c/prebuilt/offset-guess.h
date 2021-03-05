#ifndef __OFFSET_GUESS_H
#define __OFFSET_GUESS_H

#include <linux/types.h>

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

typedef struct {
    char comm[TASK_COMM_LEN];
} proc_t;

static const __u8 GUESS_SADDR = 0;
static const __u8 GUESS_DADDR = 1;
static const __u8 GUESS_FAMILY = 2;
static const __u8 GUESS_SPORT = 3;
static const __u8 GUESS_DPORT = 4;
static const __u8 GUESS_NETNS = 5;
static const __u8 GUESS_RTT = 6;
static const __u8 GUESS_DADDR_IPV6 = 7;
static const __u8 GUESS_SADDR_FL4 = 8;
static const __u8 GUESS_DADDR_FL4 = 9;
static const __u8 GUESS_SPORT_FL4 = 10;
static const __u8 GUESS_DPORT_FL4 = 11;
static const __u8 GUESS_SADDR_FL6 = 12;
static const __u8 GUESS_DADDR_FL6 = 13;
static const __u8 GUESS_SPORT_FL6 = 14;
static const __u8 GUESS_DPORT_FL6 = 15;

static const __u8 TRACER_STATE_UNINITIALIZED = 0;
static const __u8 TRACER_STATE_CHECKING = 1;
static const __u8 TRACER_STATE_CHECKED = 2;
static const __u8 TRACER_STATE_READY = 3;

typedef struct {
    __u64 state;
    // tcp_info_kprobe_status records if the tcp_info kprobe has been triggered.
    // 0 - not triggered 1 - triggered
    __u64 tcp_info_kprobe_status;

    /* checking */
    proc_t proc;
    __u64 what;
    __u64 offset_saddr;
    __u64 offset_daddr;
    __u64 offset_sport;
    __u64 offset_dport;
    __u64 offset_netns;
    __u64 offset_ino;
    __u64 offset_family;
    __u64 offset_rtt;
    __u64 offset_rtt_var;
    __u64 offset_daddr_ipv6;
    __u64 offset_saddr_fl4;
    __u64 offset_daddr_fl4;
    __u64 offset_sport_fl4;
    __u64 offset_dport_fl4;
    __u64 offset_saddr_fl6;
    __u64 offset_daddr_fl6;
    __u64 offset_sport_fl6;
    __u64 offset_dport_fl6;

    __u64 err;

    __u32 daddr_ipv6[4];
    __u32 netns;
    __u32 rtt;
    __u32 rtt_var;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u16 family;
    __u32 saddr_fl4;
    __u32 daddr_fl4;
    __u16 sport_fl4;
    __u16 dport_fl4;
    __u32 saddr_fl6[4];
    __u32 daddr_fl6[4];
    __u16 sport_fl6;
    __u16 dport_fl6;

    __u8 ipv6_enabled;
    __u8 fl4_offsets;
    __u8 fl6_offsets;
} tracer_status_t;

#endif //__OFFSET_GUESS_H
