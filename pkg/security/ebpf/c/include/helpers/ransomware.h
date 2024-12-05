#ifndef __RANSOMWARE_H__
#define __RANSOMWARE_H__

#include "maps.h"
#include "perf_ring.h"
#include "events.h"
#include "constants/macros.h"

#define RANSOMWARE_SCORE_NEW_FILE 1
#define RANSOMWARE_SCORE_UNLINK 10
#define RANSOMWARE_SCORE_RENAME 10
#define RANSOMWARE_SCORE_URANDOM 100
#define RANSOMWARE_SCORE_KILL 100

#define RANSOMWARE_WATCH_PERIOD_NS SEC_TO_NS(1)
#define RANSOMWARE_THRESHOLD_SCORE 500

__attribute__((always_inline)) void ransomware_cleanup(u32 pid) {
    bpf_map_lookup_elem(&ransomware_score, &pid);
    return;
}

__attribute__((always_inline)) void reset_score(struct ransomware_score_t *rs) {
    if (!rs) {
        return; // should not happen
    }
    __builtin_memset(rs, 0, sizeof(*rs));
    return;
}


__attribute__((always_inline)) void compute_score(ctx_t *ctx, struct ransomware_score_t *rs) {
    u32 score = 0;
    score += rs->new_file * RANSOMWARE_SCORE_NEW_FILE;
    score += rs->unlink * RANSOMWARE_SCORE_UNLINK;
    score += rs->rename * RANSOMWARE_SCORE_RENAME;
    score += rs->urandom * RANSOMWARE_SCORE_URANDOM;
    score += rs->kill * RANSOMWARE_SCORE_KILL;

    if (score < RANSOMWARE_THRESHOLD_SCORE) {
        /* bpf_printk("-- new score: %u", score); */
        return;
    }

    u64 diff_time = rs->last_syscall - rs->first_syscall;
    bpf_printk("== THRESHOLD REACHED with score: %u in %u.%u seconds", score, NS_TO_SEC(diff_time), diff_time%1000000000);
    bpf_printk("  new_files: %u", rs->new_file);
    bpf_printk("  unlinks:   %u", rs->unlink);
    bpf_printk("  renames:   %u", rs->rename);
    bpf_printk("  urandoms:  %u", rs->urandom);
    bpf_printk("  kills:     %u\n", rs->kill);

    struct ransomware_event_t event = {
        .time_to_trigger_ns = diff_time,
        .new_file = rs->new_file,
        .unlink = rs->unlink,
        .rename = rs->rename,
        .urandom = rs->urandom,
        .kill = rs->kill,
        .score = score,
    };

    /* rs->already_notified = 1; */
    u64 last = rs->last_syscall; //DEBUG, TODO: REDO ^
    reset_score(rs); //DEBUG, TODO: REDO ^
    rs->first_syscall = last; //DEBUG, TODO: REDO ^

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_RANSOMWARE, event);
    return;
}

__attribute__((always_inline)) struct ransomware_score_t* ransomware_get_score() {
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    struct ransomware_score_t *score = bpf_map_lookup_elem(&ransomware_score, &pid);
    if (!score) {
        struct ransomware_score_t s;
        reset_score(&s);
        if (bpf_map_update_elem(&ransomware_score, &pid, &s, BPF_ANY) < 0) {
            bpf_printk("ransomware_get_score failed to update elem");
            return NULL;
        }
        score = &s;
    } else if (score->already_notified) {
        return NULL;
    }

    u64 now = bpf_ktime_get_ns();
    if (score->last_syscall + RANSOMWARE_WATCH_PERIOD_NS < now) {
        reset_score(score);
        score->first_syscall = now;
    }
    score->last_syscall = now;
    return score;
}


__attribute__((always_inline)) void ransomware_score_unlink(ctx_t *ctx) {
    struct ransomware_score_t *score = ransomware_get_score();
    if (!score) {
        return;
    }
    /* bpf_printk("++ unlink"); */
    score->unlink++;
    compute_score(ctx, score);
    return;
}

__attribute__((always_inline)) void ransomware_score_rename(ctx_t *ctx) {
    struct ransomware_score_t *score = ransomware_get_score();
    if (!score) {
        return;
    }
    /* bpf_printk("++ rename"); */
    score->rename++;
    compute_score(ctx, score);
    return;
}

__attribute__((always_inline)) void ransomware_score_open(ctx_t *ctx, int flags) {
    if ((flags & (O_TRUNC|O_CREAT)) == 0) {
        return; // only interested on file creation
    }

    struct ransomware_score_t *score = ransomware_get_score();
    if (!score) {
        return;
    }
    /* bpf_printk("++ open new file"); */
    score->new_file++;
    compute_score(ctx, score);
    return;
}

__attribute__((always_inline)) void ransomware_score_kill(ctx_t *ctx, int sig) {
    if (sig != SIGKILL && sig != SIGTERM) {
        return;
    }

    struct ransomware_score_t *score = ransomware_get_score();
    if (!score) {
        return;
    }
    /* bpf_printk("++ kill"); */
    score->kill++;
    compute_score(ctx, score);
    return;
}

#endif // __RANSOMWARE_H__
