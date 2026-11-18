# nosys.so

nosys.so provides a runtime patch to support "old" versions of docker or runc.
“Old” is approximate - think of the ones shipped with Ubuntu 16.04 or RHEL 7.

The specific issue is that newer system calls (`clone3`, `faccessat2`)
may not be present in default `seccomp` profile used by older versions
of Docker, containerd, etc. This wrapper adds `seccomp` rules to make
sure these syscalls are returning `ENOSYS` instead of `EPERM`.

## Testing
*Thanks to lenaic.huard*

Unfortunately, I’m not sure such old OS are covered by our e2e tests.

### Manual testing
Here is how I validated that your PR works well:

```
Spawn an Ubuntu 16.04 VM:
ubuntu@ip-10-1-56-50:~$ hostnamectl status
[…]
  Operating System: Ubuntu 16.04.7 LTS
[…]
```

Try to start the containerized agent without the LD_PRELOAD set (pass -e LD_PRELOAD= to docker run …):
```
ubuntu@ip-10-1-56-50:~$ docker run --rm -e LD_PRELOAD= datadog/agent-dev:aiuto-mincc-py3 agent version
runtime/cgo: pthread_create failed: Operation not permitted
SIGABRT: abort
PC=0x7f108bba3c0c m=0 sigcode=18446744073709551610

goroutine 0 gp=0x7fc6b80 m=0 mp=0x7fc95c0 [idle]:
runtime: g 0 gp=0x7fc6b80: unknown pc 0x7f108bba3c0c
stack: frame={sp:0x7ffcbef34410, fp:0x0} stack=[0x7ffcbe735000,0x7ffcbef348c0)
                   7 6 5 4  3 2 1 0   f e d c  b a 9 8  0123456789abcdef
00007ffcbef34310: 00000000 00000000  00000000 00000000  ................
[…]
goroutine 1 gp=0x4b495bf81e0 m=0 mp=0x7fc95c0 [running]:
runtime.systemstack_switch()
        runtime/asm_amd64.s:516 +0x8 fp=0x4b495c8a748 sp=0x4b495c8a738 pc=0x4a21a8
runtime.main()
        runtime/proc.go:174 +0x6d fp=0x4b495c8a7e0 sp=0x4b495c8a748 pc=0x463a4d
runtime.goexit({})
        runtime/asm_amd64.s:1771 +0x1 fp=0x4b495c8a7e8 sp=0x4b495c8a7e0 pc=0x4a3de1
[…]This was to validate that we’re well on a system that requires the nosys.so hack.
```

Try to start the containerized agent with all default (including the default LD_PRELOAD value):
```
ubuntu@ip-10-1-56-50:~$ docker run --rm datadog/agent-dev:aiuto-mincc-py3 agent version
clone3 seems blocked by the seccomp profile of an old version of docker.
load a seccomp profile to force ENOSYS.
Agent 7.87.0-devel - Meta: git.573.7d388e6 - Commit: 7d388e62 - Serialization version: v5.0.215 - Go version: go1.26.10
```

This can confirm that the nosys.so hack still works as expected.

### Testing (for coding agents)

Claude code recap for https://github.com/DataDog/datadog-agent/pull/60233

What `nosys.so` solves (quick recap)

Since glibc 2.33/2.34, faccessat() calls the faccessat2 syscall first and
pthread_create() calls clone3 first. glibc only falls back to the old syscalls
(faccessat/clone) when it gets ENOSYS. Old container runtimes (Docker <
20.10.10, containerd < 1.4.10/1.5.6, or an old runc/libseccomp on the host)
have seccomp profiles that don't know these syscalls, so they return EPERM
instead. glibc then doesn't fall back, and the agent crashes at startup with
runtime/cgo: pthread_create failed: Operation not permitted. `nosys.so`, loaded
with LD_PRELOAD, probes both syscalls. Only when one of them returns EPERM,
it installs an extra seccomp filter that turns that EPERM into ENOSYS. On
modern hosts it does nothing, so there's no overhead.

Validation of #60233 (Ubuntu 16.04 on EC2, docker-ce 20.10.7, runc 1.0.0-rc95)
- agent:7.85.4 with LD_PRELOAD= (empty) → crashes with pthread_create failed:
  Operation not permitted, so this host does need the shim
- agent:7.85.4 with the default LD_PRELOAD → OK (clone3 seems blocked… / load a
  seccomp profile to force ENOSYS.)
- datadog/agent-dev:aiuto-mincc-py3 (your Bazel-built `nosys.so`) → same messages,
  the full agent runs healthy (core agent, trace, system-probe-lite, ADP)
- Size difference: there is none on amd64 (16.3 KB release vs 17.6 KB yours).
  The 70 KB is the arm64 release build, and ~59 KB of it are zero bytes: Ubuntu 24.04's
  linker aligns the end of RELRO to aarch64's 64 KiB max page size. Same NEEDED
  (libseccomp + libc), and nosys_init is still in .init_array. One minor note: the
  Bazel build doesn't get Ubuntu's default hardening (_FORTIFY_SOURCE, -fstack-protector-strong).


OS where `nosys.so` is still needed (with the distro-packaged container runtime)
```
OS                     | Runtime                                 | End of extended support      | Agents on this OS*
-----------------------|-----------------------------------------|------------------------------|-------------------
Ubuntu 16.04           | docker.io 18.09 / docker-ce ≤ 20.10.7   | Apr 2031 (Ubuntu Pro Legacy) | ~7.6k
Debian 9               | docker-ce ≤ 19.03.15 (runc rc92)        | Jun 30, 2027 (ELTS)          | ~5.0k
Debian 10              | docker.io 18.09 (docker-ce is fine)     | Jun 30, 2029 (ELTS)          | n/a
CentOS 7               | docker 1.13.1 (extras)                  | none, EOL Jun 30, 2024       | ~29.7k
RHEL 7                 | docker 1.13.1 (extras)                  | May 31, 2029 (ELS)           | ~27.6k
Amazon Linux 2 (old)   | docker 19.03 / 20.10.7 (current AL2 ships 25.x, fine) | EOL Jun 30, 2026 | ~69k total, mostly unaffected
```

All agents on that OS version, every install method, from dd.agent_stats
(Nov 13). Our telemetry doesn't report the docker/containerd version,
so the hosts that really need the shim are a subset. Upper bound: ≤ ~61k
containerized agents on such hosts (≤ ~23k plain docker run), out of ~3.57M
containerized agents. On Kubernetes, the agent container runs unconfined by
default, so the shim only matters there with seccompDefault/RuntimeDefault.
