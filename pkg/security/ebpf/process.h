#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>

static u64 fill_process_data(struct process_data_t *data) {
    // Process data
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();

    struct nsproxy *nsproxy;
    bpf_probe_read(&nsproxy, sizeof(nsproxy), &task->nsproxy);

    struct pid_namespace *pid_ns;
    bpf_probe_read(&pid_ns, sizeof(pid_ns), &nsproxy->pid_ns_for_children);
    bpf_probe_read(&data->pidns, sizeof(data->pidns), &pid_ns->ns.inum);

    // TTY
    struct signal_struct *signal;
    bpf_probe_read(&signal, sizeof(signal), &task->signal);
    struct tty_struct *tty;
    bpf_probe_read(&tty, sizeof(tty), &signal->tty);
    bpf_probe_read_str(data->tty_name, TTY_NAME_LEN, tty->name);

    // Comm
    bpf_get_current_comm(&data->comm, sizeof(data->comm));

    // Pid & Tid
    u64 id = bpf_get_current_pid_tgid();
    data->pid = id >> 32;
    data->tid = id;

    // UID & GID
    u64 userid = bpf_get_current_uid_gid();
    data->uid = userid >> 32;
    data->gid = userid;
    return id;
}

#endif