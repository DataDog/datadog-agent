#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

static struct proc_cache_t * __attribute__((always_inline)) fill_process_data(struct process_context_t *data) {
//    // Process data
//    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
//
//    struct nsproxy *nsproxy;
//    bpf_probe_read(&nsproxy, sizeof(nsproxy), &task->nsproxy);
//
//    struct pid_namespace *pid_ns;
//    bpf_probe_read(&pid_ns, sizeof(pid_ns), &nsproxy->pid_ns_for_children);
//    bpf_probe_read(&data->pidns, sizeof(data->pidns), &pid_ns->ns.inum);
//
//    // TTY
//    struct signal_struct *signal;
//    bpf_probe_read(&signal, sizeof(signal), &task->signal);
//    struct tty_struct *tty;
//    bpf_probe_read(&tty, sizeof(tty), &signal->tty);
//    bpf_probe_read_str(data->tty_name, TTY_NAME_LEN, tty->name);

    // Comm
    bpf_get_current_comm(&data->comm, sizeof(data->comm));

    // Pid & Tid
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    // https://github.com/iovisor/bcc/blob/master/docs/reference_guide.md#4-bpf_get_current_pid_tgid
    data->pid = tgid;
    data->tid = pid_tgid;

    // UID & GID
    u64 userid = bpf_get_current_uid_gid();
    data->uid = userid >> 32;
    data->gid = userid;

    struct proc_cache_t *entry = get_pid_cache(tgid);
    if (entry) {
        data->executable = entry->executable;
    }

    return entry;
}

#endif
