#include "ktypes.h"
#include "lock_contention.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "bpf_builtins.h"
#include "map-defs.h"
#include "compiler.h"
#include <asm-generic/errno-base.h>

#define LOCK_CONTENTION_IOCTL_ID 0x70C13

BPF_HASH_MAP(map_addr_fd, struct lock_range, u32, 0);

/* .rodata */
/** Ksyms **/
static volatile const u64 bpf_map_fops = 0;
static volatile const u64 bpf_dummy_read = 0;
static volatile const u64 __per_cpu_offset = 0;
/** control data **/
static volatile const u64 num_of_ranges = 0;
static volatile const u64 log2_num_of_ranges = 0;
static volatile const u64 num_cpus = 0;

static __always_inline bool is_bpf_map(u32 fd, struct file** bpf_map_file) {
    struct file **fdarray;
    u64 fn_read;
    int err;
    u64 fops;

    struct task_struct *tsk = (struct task_struct *)bpf_get_current_task();
    if (tsk == NULL)
        return false;

    err = BPF_CORE_READ_INTO(&fdarray, tsk, files, fdt, fd);
    if (err < 0)
        return false;

    err = bpf_core_read(bpf_map_file, sizeof(struct file *), fdarray + fd);
    if (err < 0)
        return false;

    struct file *map_file = *bpf_map_file;
    if (map_file == NULL)
        return false;

    err = bpf_core_read(&fops, sizeof(struct file_operations *), &map_file->f_op);
    if (err < 0)
        return false;

    if (!fops)
        return false;

    if (bpf_map_fops) {
        if (fops != bpf_map_fops)
            return false;
    } else if (bpf_dummy_read) {
        err = bpf_core_read(&fn_read, sizeof(u64), &((struct file_operations *)fops)->read);
        if (err < 0)
            return false;

        if (fn_read != bpf_dummy_read)
            return false;
    } else {
        return false;
    }

    return true;
}

static __always_inline enum bpf_map_type get_bpf_map_type(struct bpf_map* map) {
    enum bpf_map_type mtype;
    int err;

    err = bpf_core_read(&mtype, sizeof(enum bpf_map_type), &map->map_type);
    if (err < 0)
        return BPF_MAP_TYPE_UNSPEC;

    return mtype;
}

static __always_inline u64 per_cpu_ptr(u64 ptr, u64 cpu) {
    u64 cpu_per_cpu_region;
    int err;

    err = bpf_core_read(&cpu_per_cpu_region, sizeof(u64), __per_cpu_offset + (cpu * 8));
    if (err < 0)
        return 0;

    return ptr + cpu_per_cpu_region;
}

static __always_inline int record_pcpu_freelist_locks(u32 fd, struct bpf_map* bm, u32 mapid) {
    struct pcpu_freelist freelist;
    u64 region;
    int err;

    struct bpf_htab *htab = container_of(bm, struct bpf_htab, map);

    err = bpf_core_read(&freelist, sizeof(struct pcpu_freelist), &htab->freelist);
    if (err < 0)
        return err;

    for (int i = 0; i < num_cpus; i++) {
        region = per_cpu_ptr((u64)(freelist.freelist), i);
        if (!region)
            return -EINVAL;

        struct lock_range lr_pcpu_lock = {
            .addr_start =  region,
            .range = sizeof(struct pcpu_freelist_head),
            .type = HASH_PCPU_FREELIST_LOCK,
        };

        err = bpf_map_update_elem(&map_addr_fd, &lr_pcpu_lock, &mapid, BPF_NOEXIST);
        if (err < 0)
            return err;
    }

    // this regions contains the lock htab->freelist.extralist.lock
    struct lock_range lr_global_lock = {
        .addr_start = (u64)(&htab->freelist),
        .range = sizeof(struct pcpu_freelist),
        .type = HASH_GLOBAL_FREELIST_LOCK,
    };

    err = bpf_map_update_elem(&map_addr_fd, &lr_global_lock, &mapid, BPF_NOEXIST);
    if (err < 0)
        return err;

    return 0;
}

static __always_inline int record_bucket_locks(u32 fd, struct bpf_map* bm, u32 mapid) {
    u64 buckets;
    u32 n_buckets;
    int err;

    struct bpf_htab *htab = container_of(bm, struct bpf_htab, map);

    err = bpf_core_read(&buckets, sizeof(struct bucket *), &htab->buckets);
    if (err < 0)
        return err;

    err = bpf_core_read(&n_buckets, sizeof(u32), &htab->n_buckets);
    if (err < 0)
        return err;

    u64 memsz = n_buckets * sizeof(struct bucket);
    struct lock_range lr_buckets_lock = {
        .addr_start = buckets,
        .range = memsz,
        .type = HASH_BUCKET_LOCK,
    };

    err = bpf_map_update_elem(&map_addr_fd, &lr_buckets_lock, &mapid, BPF_NOEXIST);
    if (err < 0)
        return err;

    return 0;
}

static __always_inline int pcpu_lru_locks(u32 fd, struct bpf_htab *htab, u32 mapid) {
    u64 region;
    int err;
    struct bpf_lru_list *percpu_lru;

    err = bpf_core_read(&percpu_lru, sizeof(struct bpf_lru_list *), &htab->lru.percpu_lru);
    if (err < 0)
        return err;

    for (int i = 0; i < num_cpus; i++) {
        region = per_cpu_ptr((u64)(percpu_lru), i);
        if (!region)
            return -EINVAL;

        struct lock_range lr_freelist_lock = {
            .addr_start = region,
            .range = sizeof(struct bpf_lru_list),
            .type = PERCPU_LRU_FREELIST_LOCK,
        };

        err = bpf_map_update_elem(&map_addr_fd, &lr_freelist_lock, &mapid, BPF_NOEXIST);
        if (err < 0)
            return err;
    }

    return 0;
}

static __always_inline int lru_locks(u32 fd, struct bpf_htab *htab, u32 mapid) {
    int err;
    u64 region;
    u64 lock_addr = (u64)&htab->lru.common_lru.lru_list.lock;

    struct lock_range lr_global_freelist = {
        .addr_start = lock_addr,
        .range = sizeof(raw_spinlock_t),
        .type = LRU_GLOBAL_FREELIST_LOCK,
    };

    err = bpf_map_update_elem(&map_addr_fd, &lr_global_freelist, &mapid, BPF_NOEXIST);
    if (err < 0)
        return err;

    for (int i = 0; i < num_cpus; i++) {
        region = per_cpu_ptr((u64)(&htab->lru.common_lru.local_list), i);
        if (!region)
            return -EINVAL;

        struct lock_range lr_pcpu_freelist = {
            .addr_start = region,
            .range = sizeof(struct bpf_lru_locallist),
            .type = LRU_PCPU_FREELIST_LOCK,
        };

        err = bpf_map_update_elem(&map_addr_fd, &lr_pcpu_freelist, &mapid, BPF_NOEXIST);
        if (err < 0)
            return err;
    }

    return 0;
}

static __always_inline int record_lru_locks(u32 fd, struct bpf_map* bm, u32 mapid, enum bpf_map_type mtype) {
    struct bpf_htab *htab = container_of(bm, struct bpf_htab, map);

    if (mtype == BPF_MAP_TYPE_LRU_PERCPU_HASH)
        return pcpu_lru_locks(fd, htab, mapid);

    if (mtype == BPF_MAP_TYPE_LRU_HASH)
        return lru_locks(fd, htab, mapid);

    return -EINVAL;
}

static __always_inline int record_ringbuf_locks(u32 fd, struct bpf_map *bm, u32 mapid) {
    struct bpf_ringbuf_map *ringbuf_map = container_of(bm, struct bpf_ringbuf_map, map);
    struct bpf_ringbuf *rb;
    int err;

    err = bpf_core_read(&rb, sizeof(struct bpf_ringbuf *), &ringbuf_map->rb);
    if (err < 0)
        return err;


    struct lock_range lr_rb_spinlock = {
        .addr_start = (u64)&rb->spinlock,
        .range = sizeof(spinlock_t),
        .type = RINGBUF_SPINLOCK,
    };

    err = bpf_map_update_elem(&map_addr_fd, &lr_rb_spinlock, &mapid, BPF_NOEXIST);
    if (err < 0)
        return err;

    struct lock_range lr_waitq_spinlock = {
        .addr_start = (u64)&rb->waitq,
        .range = sizeof(wait_queue_head_t),
        .type = RINGBUF_WAITQ_SPINLOCK,
    };
    err = bpf_map_update_elem(&map_addr_fd, &lr_waitq_spinlock, &mapid, BPF_NOEXIST);
    if (err < 0)
        return err;

    return 0;
}

#define HAS_HASH_MAP_LOCKS(mtype) \
    (HAS_LRU_LOCKS(mtype) \
     || (mtype == BPF_MAP_TYPE_HASH) \
     || (mtype == BPF_MAP_TYPE_PERCPU_HASH) \
     || (mtype == BPF_MAP_TYPE_HASH_OF_MAPS))

#define HAS_LRU_LOCKS(mtype) \
    ((mtype == BPF_MAP_TYPE_LRU_HASH) \
     || (mtype == BPF_MAP_TYPE_LRU_PERCPU_HASH))

#define log_and_ret_err(err) \
{ \
    log_debug("[%d] err: %d", __LINE__, err); \
    return 0; \
}

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {
    int err;
    struct bpf_map *bm;
    struct file* bpf_map_file;

    u32 cmd = PT_REGS_PARM3(ctx);
    if (cmd != LOCK_CONTENTION_IOCTL_ID)
        return 0;

    u32 fd = PT_REGS_PARM2(ctx);
    if (fd <= 2)
        log_and_ret_err(-EINVAL);

    if (!is_bpf_map(fd, &bpf_map_file))
        log_and_ret_err(-EINVAL);

    u64 *mapid_ptr = (u64 *)PT_REGS_PARM4(ctx);
    if (!mapid_ptr)
        log_and_ret_err(-EINVAL);

    u32 mapid = 0;
    err = bpf_probe_read_user(&mapid, sizeof(u32), mapid_ptr);
    if (err < 0)
        log_and_ret_err(err);

    if (mapid == 0)
        log_and_ret_err(-EINVAL);

    err = bpf_core_read(&bm, sizeof(struct bpf_map *), &bpf_map_file->private_data);
    if (err < 0)
        log_and_ret_err(err);

    if (bm == NULL)
        log_and_ret_err(-EINVAL);

    enum bpf_map_type mtype = get_bpf_map_type(bm);
    if (mtype == BPF_MAP_TYPE_UNSPEC)
        log_and_ret_err(-EINVAL);

    if (HAS_HASH_MAP_LOCKS(mtype)) {
        err = record_bucket_locks(fd, bm, mapid);
        if (err < 0)
            log_and_ret_err(err);

        err = record_pcpu_freelist_locks(fd, bm, mapid);
        if (err < 0)
            log_and_ret_err(err);
    }

    if (HAS_LRU_LOCKS(mtype)) {
        err = record_lru_locks(fd, bm, mapid, mtype);
        if (err < 0)
            log_and_ret_err(err);
    }

    if (mtype == BPF_MAP_TYPE_RINGBUF) {
        err = record_ringbuf_locks(fd, bm, mapid);
        if (err < 0)
            log_and_ret_err(err);
    }

    return 0;
}

struct tstamp_data {
    struct lock_range lr;
    u64 timestamp;
    u64 lock;
    u32 flags;
};


BPF_HASH_MAP(tstamp, int, struct tstamp_data, 0);
BPF_PERCPU_ARRAY_MAP(tstamp_cpu, struct tstamp_data, 1);
BPF_HASH_MAP(lock_stat, struct lock_range, struct contention_data, 0);
BPF_PERCPU_ARRAY_MAP(ranges, struct lock_range, 0);

int data_map_full;

static __always_inline int can_record(u64 *ctx, struct lock_range* range)
{
    u64 addr = ctx[0];

    u64 end = num_of_ranges - 1;
    u64 start = 0;

    u64 m;
    struct lock_range *test_range;
    for (int i = 0; i < log2_num_of_ranges+1; i++) {
        if (start > end)
            return false;

        m = start + ((end - start) / 2);

        test_range = bpf_map_lookup_elem(&ranges, &m);
        if (!test_range)
            return false;

        if ((addr >= test_range->addr_start) && (addr <= (test_range->addr_start + test_range->range))) {
            bpf_memcpy(range, test_range, sizeof(struct lock_range));
            return true;
        }

        if (addr < test_range->addr_start)
            end = m - 1;
        else
            start = m + 1;
    }

    return false;
}

/* lock contention flags from include/trace/events/lock.h */
#define LCB_F_SPIN	(1U << 0)
#define LCB_F_READ	(1U << 1)
#define LCB_F_WRITE	(1U << 2)

static __always_inline struct tstamp_data *get_tstamp_elem(__u32 flags) {
    u32 pid;
	struct tstamp_data *pelem;

	/* Use per-cpu array map for spinlock and rwlock */
	if (flags == (LCB_F_SPIN | LCB_F_READ) || flags == LCB_F_SPIN ||
	    flags == (LCB_F_SPIN | LCB_F_WRITE)) {
		__u32 idx = 0;

		pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
		/* Do not update the element for nested locks */
		if (pelem && pelem->lock)
			pelem = NULL;
		return pelem;
	}

	pid = bpf_get_current_pid_tgid();
	pelem = bpf_map_lookup_elem(&tstamp, &pid);
	/* Do not update the element for nested locks */
	if (pelem && pelem->lock)
		return NULL;

	if (pelem == NULL) {
		struct tstamp_data zero = {};

		if (bpf_map_update_elem(&tstamp, &pid, &zero, BPF_NOEXIST) < 0) {
			return NULL;
		}

		pelem = bpf_map_lookup_elem(&tstamp, &pid);
		if (pelem == NULL) {
			return NULL;
		}
	}

	return pelem;
}

SEC("tp_btf/contention_begin")
int tracepoint__contention_begin(u64 *ctx)
{
    struct tstamp_data *pelem;
    struct lock_range range;

    if (!can_record(ctx, &range))
    	return 0;

    pelem = get_tstamp_elem(ctx[1]);
    if (pelem == NULL)
    	return 0;

    pelem->timestamp = bpf_ktime_get_ns();
    pelem->lock = (u64)ctx[0];
    pelem->flags = (u32)ctx[1];
    bpf_memcpy(&pelem->lr, &range, sizeof(struct lock_range));

    return 0;
}

SEC("tp_btf/contention_end")
int tracepoint__contention_end(u64 *ctx)
{
    u32 pid = 0, idx = 0;
    struct tstamp_data *pelem;
    struct contention_data *data;
    u64 duration;
    bool need_delete = false;

    /*
     * Spinlocks and rwlocks do not sleep. They are acquired by
     * disabling preemption to prevent them from being schedueled
     * out while inside a critical section.
     * On the other hand sleeping locks can only be acquired in
     * preemptible task context so there is no guarantee that
     * this tracepoint will shoot on the same cpu as 'contention_begin'.
     * So we cannot use a percpu map for these lock types.
     * https://docs.kernel.org/locking/locktypes.html
     *
     * For spinlock and rwlock, it needs to get the timestamp for the
     * per-cpu map.  However, contention_end does not have the flags
     * so it cannot know whether it reads percpu or hash map.
     *
     * Try per-cpu map first and check if there's active contention.
     * If it is, do not read hash map because it cannot go to sleeping
     * locks before releasing the spinning locks.
     */
    pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
    if (pelem && pelem->lock) {
    	if (pelem->lock != ctx[0])
    		return 0;
    } else {
    	pid = bpf_get_current_pid_tgid();
    	pelem = bpf_map_lookup_elem(&tstamp, &pid);
    	if (!pelem || pelem->lock != ctx[0])
    		return 0;
    	need_delete = true;
    }

    duration = bpf_ktime_get_ns() - pelem->timestamp;
    if ((s64)duration < 0) {
    	pelem->lock = 0;
    	if (need_delete)
    		bpf_map_delete_elem(&tstamp, &pid);
    	return 0;
    }


    data = bpf_map_lookup_elem(&lock_stat, &pelem->lr);
    if (!data) {
    	if (data_map_full) {
    		pelem->lock = 0;
    		if (need_delete)
    			bpf_map_delete_elem(&tstamp, &pid);
    		return 0;
    	}

    	struct contention_data first = {
    		.total_time = duration,
    		.max_time = duration,
    		.min_time = duration,
    		.count = 1,
    		.flags = pelem->flags,
    	};
    	int err;

    	err = bpf_map_update_elem(&lock_stat, &pelem->lr, &first, BPF_NOEXIST);
    	if (err < 0) {
    		if (err == -E2BIG)
    			data_map_full = 1;
    	}

    	pelem->lock = 0;
    	if (need_delete)
    		bpf_map_delete_elem(&tstamp, &pid);
    	return 0;
    }

    __sync_fetch_and_add(&data->total_time, duration);
    __sync_fetch_and_add(&data->count, 1);

    /* FIXME: need atomic operations */
    if (data->max_time < duration)
    	data->max_time = duration;
    if (data->min_time > duration)
    	data->min_time = duration;

    pelem->lock = 0;
    if (need_delete)
    	bpf_map_delete_elem(&tstamp, &pid);
    return 0;
}

char _license[] SEC("license") = "GPL";
