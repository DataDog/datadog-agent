// available from glibc 2.25
#define MAVE_DECL_GETENTROPY 0

#define HAVE_DECL_GLOB 1
#define HAVE_DECL_MMAP 1
#define HAVE_FUNC_ATTRIBUTE_DESTRUCTOR 1
#define HAVE_DLOPEN 1

// only needed by the CLI tools we don't care about
// #undef HAVE_LIBHISTORY
// #undef HAVE_LIBREADLINE

// only available on HP-UX
// #undef HAVE_SHLLOAD

#define HAVE_STDINT_H 1

#define XML_SYSCONFDIR "/opt/datadog-agent/etc"
#define XML_THREAD_LOCAL _Thread_local
