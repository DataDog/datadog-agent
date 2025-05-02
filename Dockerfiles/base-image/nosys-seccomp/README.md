In order to fix issues with newer system calls (`clone3`, `faccessat2`) that may not be present in
default `seccomp` profile used by older versions of Docker, containerd, etc, this wrapper
adds `seccomp` rules to make sure these syscalls are returning `ENOSYS` instead of `EPERM`.
