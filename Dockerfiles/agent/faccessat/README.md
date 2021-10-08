In order to fix some API issue with the `faccessat` system call, newer
kernels are now offering a richer system call: `faccessat2`.
This improvement is made transparent by the `glibc` which hides both
system calls behind the `faccessat` `glibc` function.

The `agent` docker image updated its base image from `ubuntu:20.10` to
`ubuntu:21.04` in version `7.29.0`.
This Ubuntu upgrade resulted in an upgrade of the `glibc` to a version
that tries to use the `faccessat2` system call.

When the `faccessat` `glibc` function is invoked, the `glibc` first
tries to invoke the `faccessat2` system call. If it exists, it’s
fine. But if the kernel is too old, it would return `ENOSYS` meaning
that this system call is not implemented by this kernel and the
`glibc` would then fallback to using the older `faccessat`.

So, the `glibc` ensures transparent backward compatibility to older
kernels.

Things are getting less nice when seccomp profiles are kicking in.

If a seccomp profile allows only `faccessat` and not `faccessat2`,
when the `glibc` invokes `faccessat2`, the kernel returns `EPERM`
because it is blocked by the seccomp profile.
The `glibc` interprets this as “the syscall is implemented and
returned ‘Permission denied’. So, no need to fallback to `faccessat`”.

This issue is reported on Ubuntu bugtracker [here][1] and [here][2].

`faccessat2` has been added in the default seccomp profile of
[`docker`][3] and [`containerd`][4].
It has been added in the [`system-probe` seccomp profile][5] as well.
`runc` has also been future-proofed by [returning `ENOSYS` for unknown
system calls instead of `EPERM`][6].

But adding `faccessat2` to the seccomp profile is useless if the
`libseccomp` isn’t up-to-date.
Indeed, both `docker` and `containerd` are relying on `runc` which
leverages `libseccomp` to apply the seccomp profile (see [runc][7] and
[libseccomp][8] code).

On GKE COS, the `libseccomp` of the host doesn’t recognize
`faccessat2`.
As a consequence, even if this system call is added to the
`system-probe` seccomp profile, it is ignored and `faccessat2`
constantly returns `EPERM` instead of `ENONSYS`.

In order to support GKE COS, we create an `LD_PRELOAD` based library
that intercepts `faccessat` `glibc` calls and substitute it by an
`faccessat2`-free implementation that is just a copy of [the `glibc`
version][9] in which the `faccessat2` invocation has been striped.


[1]: https://bugs.launchpad.net/ubuntu/+source/libseccomp/+bug/1916485
[2]: https://bugs.launchpad.net/ubuntu/+source/libseccomp/+bug/1914939
[3]: https://github.com/moby/moby/commit/a18139111d8a203bd211b0861c281ebe77daccd9
[4]: https://github.com/containerd/containerd/commit/6a915a1453a5bfd859664679e1ac478a7022c7f6
[5]: https://github.com/DataDog/helm-charts/pull/289/files#diff-24aa36181b1b94b8b237936a53572cd2b260f83232f97c4b74c579974d7b992f
[6]: https://github.com/opencontainers/runc/issues/2151
[7]: https://github.com/opencontainers/runc/blob/51beb5c436b159ae2d483b219c37ecfde13b006a/libcontainer/seccomp/seccomp_linux.go#L156
[8]: https://github.com/seccomp/libseccomp-golang/blob/3879420cc921efb4f1a2d75bea84e6158ef21a78/seccomp.go#L507
[9]: https://sourceware.org/git/?p=glibc.git;a=blob;f=sysdeps/unix/sysv/linux/faccessat.c;h=13160d32499c4e581d78e451461f280a4377cf3e;hb=HEAD
