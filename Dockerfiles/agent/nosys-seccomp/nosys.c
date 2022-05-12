#include <errno.h>
#include <seccomp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

__attribute__((constructor, visibility("hidden"))) void init(void)
{
  char *syscall_list = getenv("NOSYS_SYSCALLS");
  if (syscall_list == NULL) {
    return;
  }

  // Do not modify actual env var
  syscall_list = strdup(syscall_list);

  scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
  if (ctx == NULL) {
    fputs("seccomp_init failed\n", stderr);
    goto out;
  }

  char *syscall_name = strtok(syscall_list, ",");
  while (syscall_name != NULL) {
    int syscall = seccomp_syscall_resolve_name(syscall_name);
    if (syscall == __NR_SCMP_ERROR) {
      fprintf(stderr, "Unknown syscall: %s, ignoring it\n", syscall_name);
      continue;
    }

    int rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(ENOSYS), syscall, 0);
    if (rc < 0) {
      fprintf(stderr, "seccomp_rule_add failed: %s\n", strerror(-rc));
      goto out;
    }

    syscall_name = strtok(NULL, ",");
  }

  int rc = seccomp_load(ctx);
  if (rc < 0) {
    fprintf(stderr, "seccomp_rule_add failed: %s\n", strerror(-rc));
    goto out;
  }

out:
  free(syscall_list);
  seccomp_release(ctx);
  return;
}
