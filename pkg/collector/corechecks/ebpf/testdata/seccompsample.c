/* Seccomp sample program to test seccomp tracer
 * Compile with: gcc -static -o seccompsample seccompsample.c -lseccomp
 */

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <seccomp.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/syscall.h>

// Helper functions to create distinct call stacks for testing
static void trigger_getpid_level3(void) {
    errno = 0;
    long pid = syscall(SYS_getpid);
    int saved_errno = errno;
    if (pid == -1 && saved_errno == EPERM) {
        printf("getpid() denied as expected (from level 3)\n");
    } else {
        printf("getpid() returned %ld with errno %d\n", pid, saved_errno);
    }
}

static void trigger_getpid_level2(void) {
    trigger_getpid_level3();
}

static void trigger_getpid_level1(void) {
    trigger_getpid_level2();
}

static void trigger_getuid_level2(void) {
    errno = 0;
    long uid = syscall(SYS_getuid);
    int saved_errno = errno;
    if (uid == -1 && saved_errno == EACCES) {
        printf("getuid() denied as expected (from level 2)\n");
    } else {
        printf("getuid() returned %ld with errno %d\n", uid, saved_errno);
    }
}

static void trigger_getuid_level1(void) {
    trigger_getuid_level2();
}

int main(int argc, char *argv[]) {
    int wait_time = 5; // default wait time

    if (argc > 1) {
        wait_time = atoi(argv[1]);
    }

    printf("Starting SeccompSample program\n");
    fflush(stdout);

    // Wait before setting up seccomp to allow tracer to attach
    sleep(wait_time);

    // Set up seccomp filter that denies getpid with ERRNO
    scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
    if (ctx == NULL) {
        fprintf(stderr, "Failed to initialize seccomp\n");
        return 1;
    }

    // Deny getpid syscall with ERRNO action
    int rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EPERM), SCMP_SYS(getpid), 0);
    if (rc < 0) {
        fprintf(stderr, "Failed to add seccomp rule\n");
        seccomp_release(ctx);
        return 1;
    }

    // Deny getuid syscall with ERRNO action
    rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EACCES), SCMP_SYS(getuid), 0);
    if (rc < 0) {
        fprintf(stderr, "Failed to add seccomp rule for getuid\n");
        seccomp_release(ctx);
        return 1;
    }

    // Load the seccomp filter
    rc = seccomp_load(ctx);
    if (rc < 0) {
        fprintf(stderr, "Failed to load seccomp filter: %d\n", rc);
        seccomp_release(ctx);
        return 1;
    }

    printf("Seccomp filter loaded successfully\n");
    fflush(stdout);

    seccomp_release(ctx);

    // Trigger denials from different call stacks to test stack trace capture
    printf("Triggering denials from nested functions...\n");
    fflush(stdout);

    trigger_getpid_level1();
    trigger_getuid_level1();

    printf("Seccomp denials triggered.\n");
    fflush(stdout);

    return 0;
}
