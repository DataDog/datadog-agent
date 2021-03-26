/*
Patch clang/llvm to remove references to glibc symbols with a too recent version.

Except for the glibc which is not packaged with the rest. We expect to use
the glibc shipped with the system.

We are currently facing compatibility issues with old distributions.

Here is the error we get on CentOS 7 when trying to start system-probe:
```
[root@qa-linux-agent6-unstable-centos7-node-01 datadog]# /opt/datadog-agent/embedded/bin/system-probe --config=/etc/datadog-agent/system-probe.yaml --pid=/opt/datadog-agent/run/system-probe.pid
/opt/datadog-agent/embedded/bin/system-probe: /lib64/libm.so.6: version `GLIBC_2.29' not found
/opt/datadog-agent/embedded/bin/system-probe: /lib64/libc.so.6: version `GLIBC_2.26' not found
```

The reference to `GLIBC_2.29` comes from the mathematical functions `exp`,
`log`, `pow`, `exp2` and `log2`.
Fortunately, the glibc also provides older versions of those function.
So, the fix consists in using the `GLIBC_2.2.5` version of those symbols
instead of the `GLIBC_2.29` version one.

Commands used to find symbols requiring a new version of GLIBC:
$ inv -e system-probe.build
// see version requirements at end of output
$ objdump -p bin/system-probe/system-probe
// figure out which functions/symbols need that version
$ nm bin/system-probe/system-probe | grep GLIBC_X.XX
*/

#ifdef __GLIBC__

#ifdef __x86_64__
#define GLIBC_VERS "GLIBC_2.2.5"
#elif defined(__aarch64__)
#define GLIBC_VERS "GLIBC_2.17"
#else
#error Unknown architecture
#endif

#define symver_wrap_d1(func)                                    \
double __ ## func ## _prior_glibc(double x);                    \
                                                                \
asm(".symver __" #func "_prior_glibc, " #func "@" GLIBC_VERS);  \
                                                                \
double __wrap_ ## func (double x) {                             \
  return __ ## func ## _prior_glibc(x);                         \
}

#define symver_wrap_d2(func)                                    \
double __ ## func ## _prior_glibc(double x, double y);          \
                                                                \
asm(".symver __" #func "_prior_glibc, " #func "@" GLIBC_VERS);  \
                                                                \
double __wrap_ ## func (double x, double y) {                   \
  return __ ## func ## _prior_glibc(x, y);                      \
}

#define symver_wrap_f1(func)                                    \
float __ ## func ## _prior_glibc(float x);                      \
                                                                \
asm(".symver __" #func "_prior_glibc, " #func "@" GLIBC_VERS);  \
                                                                \
float __wrap_ ## func (float x) {                               \
  return __ ## func ## _prior_glibc(x);                         \
}

#else

// Use functions directly for non-GLIBC environments.

#define symver_wrap_d1(func)                                    \
double func(double x);                                          \
                                                                \
double __wrap_ ## func (double x) {                             \
  return func(x);                                               \
}

#define symver_wrap_d2(func)                                    \
double func(double x, double y);                                \
                                                                \
double __wrap_ ## func (double x, double y) {                   \
  return func(x, y);                                            \
}

#define symver_wrap_f1(func)                                    \
float func(float x);                                            \
                                                                \
float __wrap_ ## func (float x) {                               \
  return func(x);                                               \
}

#endif


symver_wrap_d1(exp)
symver_wrap_d1(log)
symver_wrap_d2(pow)
symver_wrap_d1(log2)
symver_wrap_f1(log2f)
