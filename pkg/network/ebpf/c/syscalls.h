#ifndef __SYSCALLS_H
#define __SYSCALLS_H

#include <linux/kconfig.h>
#include <net/sock.h>

/*
    field:int __syscall_nr;	            offset:8;	size:4;	signed:1;
	field:int fd;	                    offset:16;	size:8;	signed:0;
	field:struct sockaddr * umyaddr;	offset:24;	size:8;	signed:0;
	field:int addrlen;	                offset:32;	size:8;	signed:0;
 */
struct syscalls_enter_bind_args {
    __u64 unused;
    __s32 syscall_nr;
    __u32 pad;

    __u64 fd;
    struct sockaddr *umyaddr;
    __u64 addrlen;
};

/*
    field:int __syscall_nr;	offset:8;	size:4;	signed:1;
	field:long ret;	        offset:16;	size:8;	signed:1;
 */
struct syscalls_exit_args {
    __u64 unused;
    __s32 syscall_nr;
    __u32 pad;

    __s64 ret;
};

/*
    field:int __syscall_nr;	offset:8;	size:4;	signed:1;
	field:int family;       offset:16;	size:8;	signed:0;
	field:int type;	        offset:24;	size:8;	signed:0;
	field:int protocol;	    offset:32;	size:8;	signed:0;
 */
struct syscalls_enter_socket_args {
    __u64 unused;
    __s32 syscall_nr;
    __u32 pad;

    __u64 family;
    __u64 type;
    __u64 protocol;
};

#endif //__SYSCALLS_H
