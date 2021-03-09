/*
Patch clang/llvm to remove references to glibc symbols with a too recent version.

See libbcc_compat.patch for the reason behind this patch.
Of note, we do not have the problem with reallocarray.

Commands used to find symbols requiring a new version of GLIBC:
// build without BCC so it doesn't cloud requirements
$ inv -e system-probe.build --no-with-bcc
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
