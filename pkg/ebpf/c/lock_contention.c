#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "compiler.h"

#define LOCK_CONTENTION_IOCTL_ID 0x70C13

struct lock_range {
    u64 addr_start;
    u64 range;
};

BPF_HASH_MAP(map_fd_addr, u32, struct lock_range, 0);

/* error stats */
int update_failed;
int update_success;


static volatile const u64 bpf_map_fops = 0; // .rodata

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {
    struct file **fdarray;
    int err;
    struct file* bpf_map_file;
    u64 fops;
    struct bpf_map *bm;
    u64 buckets;
    u32 n_buckets;

    u32 cmd = PT_REGS_PARM3(ctx);
    if (cmd != LOCK_CONTENTION_IOCTL_ID) {
        return 0;
    }

    u32 fd = PT_REGS_PARM2(ctx);
    if (fd <= 2)
        return 0;

    struct task_struct *tsk = (struct task_struct *)bpf_get_current_task();
    if (tsk == NULL)
        return 0;

    err = BPF_CORE_READ_INTO(&fdarray, tsk, files, fdt, fd);
    if (err < 0)
        return 0;

    err = bpf_core_read(&bpf_map_file, sizeof(struct file *), fdarray + fd);
    if (err < 0)
        return 0;

    if (bpf_map_file == NULL)
        return 0;

    err = bpf_core_read(&fops, sizeof(struct file_operations *), &bpf_map_file->f_op);
    if (err < 0)
        return 0;

    if (!fops)
        return 0;

    if (fops != bpf_map_fops)
        return 0;

    err = bpf_core_read(&bm, sizeof(struct bpf_map *), &bpf_map_file->private_data);
    if (err < 0)
        return 0;

    if (bm == NULL)
        return 0;

    struct bpf_htab *htab = container_of(bm, struct bpf_htab, map);


    err = bpf_probe_read(&buckets, sizeof(struct bucket *), &htab->buckets);
    if (err < 0)
        return 0;

    err = bpf_probe_read(&n_buckets, sizeof(u32), &htab->n_buckets);
    if (err < 0)
        return 0;

    u64 memsz = n_buckets * sizeof(struct bucket);
    struct lock_range lr = { .addr_start = buckets, .range = memsz};

    err = bpf_map_update_elem(&map_fd_addr, &fd, &lr, BPF_NOEXIST);
    if (err < 0) {
        // no need for atomic operation since this bpf program is called serially
        update_failed++;
        return 0;
    }

    update_success++;

    return 0;
}

char _license[] SEC("license") = "GPL";
