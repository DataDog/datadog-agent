// SPDX-License-Identifier: Apache-2.0
// Standalone perf-uprobe release reproducer. Only probes this executable.
#define _GNU_SOURCE
#include <errno.h>
#include <dirent.h>
#include <linux/bpf.h>
#include <linux/perf_event.h>
#include <pthread.h>
#include <signal.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/resource.h>
#include <sys/syscall.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

static int *events, *links, count, resources;
static atomic_int cursor;

// Distinct valid instruction offsets, never executed by the workload.
__asm__(".pushsection .text\n.balign 16\n.global probe_targets\nprobe_targets:\n.rept 10000\nnop\n.endr\n.popsection\n");
extern unsigned char probe_targets[];
#if defined(__aarch64__)
#define INSTRUCTION_BYTES 4
#elif defined(__x86_64__)
#define INSTRUCTION_BYTES 1
#else
#error Unsupported architecture
#endif
static void die(const char *what) { perror(what); exit(1); }
static double now(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return ts.tv_sec + ts.tv_nsec / 1e9;
}
static unsigned long target_offset(void) {
    FILE *f = fopen("/proc/self/maps", "r");
    if (!f) die("maps");
    char line[8192];
    unsigned long start, end, off, addr = (unsigned long)probe_targets;
    while (fgets(line, sizeof(line), f)) {
        if (sscanf(line, "%lx-%lx %*s %lx", &start, &end, &off) == 3 && addr >= start && addr < end) {
            fclose(f);
            return addr - start + off;
        }
    }
    fprintf(stderr, "target mapping not found\n"); exit(1);
}
static void *release(void *unused) {
    (void)unused;
    for (;;) {
        int i = atomic_fetch_add(&cursor, 1);
        if (i >= resources) return NULL;
        if (links[i] >= 0 && close(links[i])) die("close link");
        if (events[i] >= 0 && close(events[i])) die("close perf event");
    }
}
static void sample_stacks(pid_t pid) {
    char tasks[128];
    snprintf(tasks, sizeof(tasks), "/proc/%d/task", pid);
    DIR *dir = opendir(tasks);
    if (!dir) return;
    struct dirent *ent;
    while ((ent = readdir(dir))) {
        if (ent->d_name[0] == '.') continue;
        char path[512], line[512];
        snprintf(path, sizeof(path), "%s/%s/stack", tasks, ent->d_name);
        FILE *f = fopen(path, "r");
        if (!f) continue;
        fprintf(stderr, "STACK pid=%d tid=%s time=%.6f\n", pid, ent->d_name, now());
        while (fgets(line, sizeof(line), f)) fputs(line, stderr);
        fclose(f);
    }
    closedir(dir);
}
static int supervise(const char *exe, const char *n, const char *workers, int sig) {
    int channel[2];
    if (pipe(channel)) die("pipe");
    pid_t child = fork();
    if (child < 0) die("fork");
    if (!child) {
        close(channel[0]); dup2(channel[1], STDOUT_FILENO); close(channel[1]);
        const char *scenario = getenv("DD_UPROBE_SHUTDOWN_EXEC");
        if (scenario) execl(scenario, scenario, "-test.run=^TestUprobeShutdownScenario$", "-test.v", NULL);
        else execl(exe, exe, n, workers, NULL);
        die("exec");
    }
    close(channel[1]);
    FILE *output = fdopen(channel[0], "r");
    char line[8192];
    for (;;) {
        if (!fgets(line, sizeof(line), output)) {
            int status; waitpid(child, &status, 0); return 1;
        }
        fputs(line, stdout); fflush(stdout);
        if (strstr(line, "\"phase\":\"ready\"")) break;
    }
    double start = now(), next_sample = start;
    if (kill(child, sig)) die("kill");
    int status;
    while (waitpid(child, &status, WNOHANG) == 0) {
        if (now() >= next_sample) { sample_stacks(child); next_sample = now()+0.25; }
        struct timespec pause = {0, 10000000}; nanosleep(&pause, NULL);
    }
    double elapsed = now()-start;
    while (fgets(line, sizeof(line), output)) fputs(line, stdout);
    fclose(output);
    printf("{\"phase\":\"reaped\",\"probes\":%s,\"mode\":\"%s\",\"signal\":%d,\"exit_seconds\":%.6f,\"status\":%d}\n", n, workers, sig, elapsed, status);
    fflush(stdout);
    return sig == SIGKILL ? !(WIFSIGNALED(status) && WTERMSIG(status) == SIGKILL) : !(WIFEXITED(status) && WEXITSTATUS(status) == 0);
}
int main(int argc, char **argv) {
    if (argc == 5 && !strcmp(argv[1], "--supervise"))
        return supervise(argv[0], argv[2], argv[3], !strcmp(argv[4], "kill") ? SIGKILL : SIGTERM);
    if (argc != 3) { fprintf(stderr, "usage: %s PROBES WORKERS\n", argv[0]); return 2; }
    count = atoi(argv[1]);
    int multi = !strncmp(argv[2], "multi:", 6);
    int perf_link = !strncmp(argv[2], "link:", 5);
    int group = multi ? atoi(argv[2]+6) : 1;
    int workers = multi ? 1 : atoi(argv[2] + (perf_link ? 5 : 0));
    if (count < 1 || count > 10000 || workers < 1 || workers > 64 || group < 1) return 2;
    struct rlimit lim = {RLIM_INFINITY, RLIM_INFINITY};
    setrlimit(RLIMIT_MEMLOCK, &lim);
    lim.rlim_cur = lim.rlim_max = 65536;
    if (setrlimit(RLIMIT_NOFILE, &lim)) die("rlimit files");
    sigset_t signals;
    sigemptyset(&signals); sigaddset(&signals, SIGTERM); sigaddset(&signals, SIGINT);
    if (pthread_sigmask(SIG_BLOCK, &signals, NULL)) die("signal mask");
    char verifier[65536] = {0};
    struct bpf_insn insns[] = {
        {.code = BPF_ALU64 | BPF_MOV | BPF_K, .dst_reg = BPF_REG_0, .imm = 0},
        {.code = BPF_JMP | BPF_EXIT},
    };
    union bpf_attr load = {0};
    load.prog_type = BPF_PROG_TYPE_KPROBE;
    if (multi) load.expected_attach_type = BPF_TRACE_UPROBE_MULTI;
    load.insn_cnt = 2; load.insns = (uintptr_t)insns;
    load.license = (uintptr_t)"GPL";
    load.log_buf = (uintptr_t)verifier; load.log_size = sizeof(verifier); load.log_level = 1;
    strcpy(load.prog_name, "exit_repro");
    int prog = syscall(SYS_bpf, BPF_PROG_LOAD, &load, sizeof(load));
    if (prog < 0) { fprintf(stderr, "%s\n", verifier); die("BPF_PROG_LOAD"); }
    FILE *f = fopen("/sys/bus/event_source/devices/uprobe/type", "r");
    int type;
    if (!f || fscanf(f, "%d", &type) != 1) die("uprobe PMU");
    fclose(f);
    char path[4096];
    ssize_t len = readlink("/proc/self/exe", path, sizeof(path)-1);
    if (len < 0) die("exe");
    path[len] = 0;
    struct perf_event_attr attr = {0};
    attr.size = sizeof(attr); attr.type = type;
    attr.config1 = (uintptr_t)path; attr.config2 = target_offset();
    events = calloc(count, sizeof(int)); links = calloc(count, sizeof(int));
    if (!events || !links) die("calloc");
    double start = now();
    unsigned long base = target_offset();
    if (multi) {
        uint64_t *offsets = calloc(count, sizeof(uint64_t));
        if (!offsets) die("offsets");
        for (int i=0; i<count; i++) offsets[i] = base + i*INSTRUCTION_BYTES;
        for (int i=0; i<count; i+=group) {
            union bpf_attr attach = {0};
            attach.link_create.prog_fd = prog;
            attach.link_create.attach_type = BPF_TRACE_UPROBE_MULTI;
            attach.link_create.uprobe_multi.path = (uintptr_t)path;
            attach.link_create.uprobe_multi.offsets = (uintptr_t)(offsets+i);
            attach.link_create.uprobe_multi.cnt = count-i < group ? count-i : group;
            links[resources] = syscall(SYS_bpf, BPF_LINK_CREATE, &attach, sizeof(attach));
            if (links[resources] < 0) die("BPF_LINK_CREATE multi");
            events[resources++] = -1;
        }
        free(offsets);
    } else for (int i = 0; i < count; i++) {
        // CPU 0, all tasks, but only the private executable's target instruction.
        attr.config2 = base + i*INSTRUCTION_BYTES;
        events[i] = syscall(SYS_perf_event_open, &attr, -1, 0, -1, PERF_FLAG_FD_CLOEXEC);
        if (events[i] < 0) { fprintf(stderr, "probe %d\n", i); die("perf_event_open"); }
        links[i] = -1;
        if (perf_link) {
            union bpf_attr attach = {0};
            attach.link_create.prog_fd = prog; attach.link_create.target_fd = events[i];
            attach.link_create.attach_type = BPF_PERF_EVENT;
            links[i] = syscall(SYS_bpf, BPF_LINK_CREATE, &attach, sizeof(attach));
            if (links[i] < 0) die("BPF_LINK_CREATE");
        } else {
            // ebpf-manager v0.8.7 uses SET_BPF + ENABLE, not BPF perf links.
            if (ioctl(events[i], PERF_EVENT_IOC_SET_BPF, prog)) die("PERF_EVENT_IOC_SET_BPF");
            if (ioctl(events[i], PERF_EVENT_IOC_ENABLE, 0)) die("PERF_EVENT_IOC_ENABLE");
        }
        resources++;
    }
    printf("{\"phase\":\"ready\",\"pid\":%d,\"probes\":%d,\"links\":%d,\"mode\":\"%s\",\"attach_seconds\":%.6f}\n", getpid(), count, resources, argv[2], now()-start);
    fflush(stdout);
    int received;
    if (sigwait(&signals, &received)) die("sigwait");
    start = now();
    printf("{\"phase\":\"cleanup_start\"}\n"); fflush(stdout);
    pthread_t threads[64];
    for (int i = 0; i < workers; i++) if (pthread_create(&threads[i], NULL, release, NULL)) die("pthread_create");
    for (int i = 0; i < workers; i++) if (pthread_join(threads[i], NULL)) die("pthread_join");
    if (close(prog)) die("close prog");
    printf("{\"phase\":\"cleanup_done\",\"cleanup_seconds\":%.6f}\n", now()-start);
    return 0;
}
