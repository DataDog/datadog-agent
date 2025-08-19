#define _GNU_SOURCE // Needed for the TEMP_FAILURE_RETRY macro.
#include <stdlib.h>
#include <stdbool.h>
#include <stdio.h>
#include <dirent.h>
#include <fcntl.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/syscall.h>
#include <sys/wait.h>
#include <linux/wait.h>
#include <linux/sched.h>
#include <seccomp.h>

static void cleanup_fd(int *dirfd) {
    if (close(*dirfd) < 0) {
        perror("Failed to close \"/\"");
    }
}

static bool test_faccessat2(void) {
    int dirfd __attribute__((__cleanup__(cleanup_fd))) = TEMP_FAILURE_RETRY(open("/", O_CLOEXEC | O_DIRECTORY));
    if (dirfd < 0)
        perror("failed to open \"/\"");

    long rc = syscall(SYS_faccessat2, dirfd, "/", F_OK, 0);

    // Success or not implemented
    if (rc == 0 || errno == ENOSYS)
        return true;

    // Blocked by docker seccomp profile
    if (rc < 0 && errno == EPERM)
        return false;

    perror("failed to faccessat2");
    return true;
}

static bool test_clone3(void) {
    int child_pidfd;
    struct clone_args cl_args = {
        .exit_signal = SIGCHLD,
        .flags = CLONE_PIDFD,
        .pidfd = (uint64_t)&child_pidfd
    };

    long rc = syscall(SYS_clone3, &cl_args, sizeof(cl_args));

    // Child process
    if (rc == 0)
        exit(EXIT_SUCCESS);

    // Success
    else if (rc > 0) {
        siginfo_t infop;
        if (TEMP_FAILURE_RETRY(waitid(P_PIDFD, child_pidfd, &infop, WEXITED)) < 0)
            perror("failed to waitid");
        return true;
    }

    // Not implemented
    else if (rc < 0 && errno == ENOSYS)
        return true;

    // Blocked by docker seccomp profile
    if (rc < 0 && errno == EPERM)
        return false;

    perror("failed to clone3");
    return true;
}

static void cleanup_seccomp_ctx(scmp_filter_ctx *ctx) {
    seccomp_release(*ctx);
}

__attribute__((constructor, visibility("hidden"))) void nosys_init(void) {
    bool faccessat2_is_usable = test_faccessat2();
    bool clone3_is_usable = test_clone3();

    if (faccessat2_is_usable && clone3_is_usable)
        return;

    scmp_filter_ctx ctx __attribute__((cleanup(cleanup_seccomp_ctx))) = seccomp_init(SCMP_ACT_ALLOW);
    if (ctx == NULL) {
        fputs("seccomp_init failed\n", stderr);
        return;
    }

    if (!faccessat2_is_usable) {
        fputs("faccessat2 seems blocked by the seccomp profile of an old version of docker.\n", stderr);
        int rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(ENOSYS), SCMP_SYS(faccessat2), 0);
        if (rc < 0)
            fprintf(stderr, "seccomp_rule_add failed: %s\n", strerror(-rc));
    }

    if (!clone3_is_usable) {
        fputs("clone3 seems blocked by the seccomp profile of an old version of docker.\n", stderr);
        int rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(ENOSYS), SCMP_SYS(clone3), 0);
        if (rc < 0)
            fprintf(stderr, "seccomp_rule_add failed: %s\n", strerror(-rc));
    }

    fputs("load a seccomp profile to force ENOSYS.\n", stderr);
    int rc = seccomp_load(ctx);
    if (rc < 0) {
        fprintf(stderr, "seccomp_load failed: %s\n", strerror(-rc));
        return;
    }
}
