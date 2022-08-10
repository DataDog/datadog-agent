#include <linux/kconfig.h>
#include "pid_mapper.h"
#include <linux/net.h>
#include <linux/fs.h>
#include <linux/fdtable.h>
#include <linux/sched.h>
#include <net/sock.h>
#include <linux/socket.h>

#define SOCKET_INODE_OPS_ID 1
#define TCP_OPS_ID 2
#define INET_OPS_ID 3

struct bpf_map_def SEC("maps/save_pid") save_pid = {
   .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(int),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/symbol_table") symbol_table = {
   .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(__u32),
    .max_entries = 3,
    .pinning = 0,
    .namespace = "",
};


#define KERNEL_READ_FAIL(dest, sz, src)\
    do {                                            \
    if (bpf_probe_read_kernel(dest, sz, src) < 0)   \
        return 0;                                   \
    } while(0);

/* The following hooks are used to form a mapping for the struct sock*
 * objects created before system probe was started. Userspace triggers
 * the ebpf program by interacting with procfs. These hooks will be removed
 * by the userspace program once it has walked all the pids in procfs.
 * kprobe/user_path_at_empty: filters for procfs events and parses the pid
 * kprobe/d_path: perform the sock->pid mapping */

// prefix: /proc/
#define FDPATH_SZ 32
#define PREFIX_END 6
#define MAX_UINT_LEN 10
static int __always_inline parse_and_check_path(char* buffer) {
    // /proc/<MAX_UINT_LEN>/fd/<MAX_UINT_LEN>
    char *pidptr = buffer+PREFIX_END;
    int pid = 0;

    if (!((buffer[0] == '/') && (buffer[1] == 'p') && (buffer[2] == 'r') && (buffer[3] == 'o') && (buffer[4] == 'c') && (buffer[5] == '/')))
        return -1;

#pragma unroll
    for (int i = 0; i < MAX_UINT_LEN; i++) {
        if (*pidptr == '/')
            break;
        
        if ((*pidptr < '0') || (*pidptr > '9'))
            return -1;

        pid = (*pidptr - '0') + (pid * 10);

        pidptr++;
    }

    if (!((pidptr[1] == 'f') && (pidptr[2] == 'd') && (pidptr[3] == '/')))
        return -1;

    pidptr += 4;

    for (int i = 0; i < MAX_UINT_LEN; ++i) {
        if (pidptr[i] == 0)
            return pid;

        if ((pidptr[i] < '0') || (pidptr[i] > '9'))
            return -1;
    }


    return pid;
}

SEC("kprobe/user_path_at_empty")
int kprobe__user_path_at_empty(struct pt_regs* ctx) {
    char* path = (char *)PT_REGS_PARM2(ctx);
    char buffer[FDPATH_SZ];
    if (path == 0)
        return 0;

    if (bpf_probe_read_user(&buffer, FDPATH_SZ, path) < 0)
        return 0;

    int pid = parse_and_check_path(buffer);
    log_info("buffer: %s\n", buffer);
    log_info("pid: %d\n", pid);
    if (pid == -1)
        return 0;

    u64 tgidpid = bpf_get_current_pid_tgid();

    bpf_map_update_elem(&save_pid, &tgidpid, &pid, BPF_NOEXIST);

    return 0;
}

static __always_inline void map_sock_to_pid(struct socket* sock, u32 pid) {
    struct sock* sk;

    bpf_probe_read_kernel(&sk, sizeof(struct sock *), &sock->sk);
    if (sk == NULL)
        return;

    bpf_map_update_elem(&sock_to_pid, &sk, &pid, BPF_NOEXIST);
}

static __always_inline int fingerprint_tcp_inet_ops(struct socket* sock) {
    struct proto_ops *pops;

    KERNEL_READ_FAIL(&pops, sizeof(struct proto_ops *), &sock->ops);
    if (!pops)
        return 0;

    u32 *addr_id = bpf_map_lookup_elem(&symbol_table, &pops);
    if (!addr_id)
        return 0;

    if ((*addr_id == TCP_OPS_ID) || (*addr_id == INET_OPS_ID)) {
        return 1;
    }

    return 0;
}

static __always_inline int is_socket_inode(struct inode* inode) {
    struct inode_operations* i_op;

    KERNEL_READ_FAIL(&i_op, sizeof(struct inode_operations *), &inode->i_op);
    if (!i_op)
        return 0;
    
    // The inode_operations of a file wrapping a struct socket object
    // are allocated here: sock_alloc(): https://elixir.bootlin.com/linux/v4.4/source/net/socket.c#L552
    // We check the if the pointer is to the sockfs_inode_ops object to fingerprint
    // a socket inode.
    u32 *addr_id = bpf_map_lookup_elem(&symbol_table, &i_op);
    if (!addr_id)
        return 0;

    return *addr_id == SOCKET_INODE_OPS_ID;
}

static __always_inline struct socket *get_socket_from_dentry(struct dentry *dentry) {
    struct inode* inode;

    KERNEL_READ_FAIL(&inode, sizeof(struct inode *), &dentry->d_inode);
    if (!inode)
        return 0;

    if (!is_socket_inode(inode))
        return 0;

    // The struct socket and struct inode are allocated together as a tuple and wrapped
    // inside a struct socket_alloc object. 
    // See sock_alloc_inode(): https://elixir.bootlin.com/linux/latest/source/net/socket.c#L300
    return (struct socket *)container_of(inode, struct socket_alloc, vfs_inode);

 }

SEC("kprobe/d_path")
int kprobe__d_path(struct pt_regs* ctx) {
    struct dentry* dentry;
    struct socket* sock;
    
    struct path* path = (struct path *)PT_REGS_PARM1(ctx);
    u64 tgid = bpf_get_current_pid_tgid();
    int* pidptr = bpf_map_lookup_elem(&save_pid, &tgid);
    if (!pidptr)
        return 0;

    int pid = *pidptr;
    bpf_map_delete_elem(&save_pid, &tgid);

    KERNEL_READ_FAIL(&dentry, sizeof(struct dentry *), &path->dentry);
    if (!dentry)
        return 0;

    sock = get_socket_from_dentry(dentry);
    if (!sock)
        return 0;

    if (!fingerprint_tcp_inet_ops(sock))
        return 0;

    map_sock_to_pid(sock, pid);

    return 0;
}

/* The following hooks are used to track the lifecycle of the process */

// The audit context is used by the audit subsystem of the kernel
// It is set per task on every syscall entry in the sysenter tracepoint
// We use in_syscall field to filter for events originating from userspace.
struct audit_context {
    int dummy;
    int in_syscall;
};

static __always_inline int is_syscall_ctx(struct task_struct *tsk) {
    int in_syscall;
    struct audit_context* actx;

    KERNEL_READ_FAIL(&actx, sizeof(struct audit_context *), &tsk->audit_context);
    if (!actx)
        return 0;

    KERNEL_READ_FAIL(&in_syscall, sizeof(int), &actx->in_syscall);
    return in_syscall;
}


SEC("kprobe/security_sk_alloc")
int kprobe__security_sk_alloc(struct pt_regs *ctx) {
    struct sock* sk = (struct sock *)PT_REGS_PARM1(ctx);
    if (!sk)
        return 0;

    struct task_struct *tsk = (struct task_struct *)bpf_get_current_task();
    if (tsk == NULL)
        return 0;

    int family = PT_REGS_PARM2(ctx);
    if (!((family == AF_INET) || (family == AF_INET6)))
        return 0;

    if (!is_syscall_ctx(tsk))
        return 0;

    u64 tgid = bpf_get_current_pid_tgid() >> 32;
    bpf_map_update_elem(&sock_to_pid, &sk, &tgid, BPF_NOEXIST);

    return 0;
}

SEC("kprobe/security_sk_clone")
int kprobe__security_sk_clone(struct pt_regs *ctx) {
    struct sock* sk = (struct sock *)PT_REGS_PARM2(ctx);
    if (sk == NULL)
        return 0;

    struct task_struct *tsk = (struct task_struct *)bpf_get_current_task();
    if (tsk == NULL)
        return 0;

    if (!is_syscall_ctx(tsk))
        return 0;

    u64 tgid = bpf_get_current_pid_tgid() >> 32;

    bpf_map_update_elem(&sock_to_pid, &sk, &tgid, BPF_NOEXIST);

    return 0;
}

SEC("kprobe/security_sk_free")
int kprobe__security_sk_free(struct pt_regs* ctx) {
    struct sock* sk = (struct sock *)PT_REGS_PARM1(ctx);
    if (sk == NULL)
        return 0;

    bpf_map_delete_elem(&sock_to_pid, &sk);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)
char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
