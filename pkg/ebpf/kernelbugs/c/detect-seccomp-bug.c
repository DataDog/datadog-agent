/* Program to test whether seccomp allows uretprobe syscalls
 * Compile with: gcc detect-seccomp-bug -o bug -lseccomp
 */

#include <stdio.h>
#include <seccomp.h>
#include <signal.h>
#include <stdlib.h>

char *syscalls[] = {
    "write",
    "exit_group",
    "close",
    "fstat",
    "prctl",
};

__attribute__((noinline)) int trigger_uretprobe_syscall(void) {
    return 0;
}

void segv_handler(int code) {
    exit(code);
}

void apply_seccomp_filter(char **syscalls, int num_syscalls) {
    scmp_filter_ctx ctx;

    ctx = seccomp_init(SCMP_ACT_ERRNO(1));
    for (int i = 0; i < num_syscalls; i++) {
        seccomp_rule_add(ctx, SCMP_ACT_ALLOW, seccomp_syscall_resolve_name(syscalls[i]), 0);
    }
    seccomp_load(ctx);
    seccomp_release(ctx);
}

int main(int argc, char *argv[])
{
    struct sigaction act = { 0 };
    act.sa_handler = &segv_handler;

    if (sigaction(SIGSEGV, &act, NULL) == -1) {
        exit(1);
    }

    int num_syscalls = sizeof(syscalls) / sizeof(syscalls[0]);
    apply_seccomp_filter(syscalls, num_syscalls);

    trigger_uretprobe_syscall();

    return 0;
}
