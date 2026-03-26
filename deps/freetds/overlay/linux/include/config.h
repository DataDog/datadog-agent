/* include/config.h.  Generated from config.h.in by configure.  */
/* include/config.h.in.  Generated from configure.ac by autoheader.  */

#define _freetds_config_h_

/* Define if building universal (internal helper macro) */
/* #undef AC_APPLE_UNIVERSAL_BUILD */

/* Define to 1 if you need BSD_COMP defined to get FIONBIO defined. */
/* #undef BSD_COMP */

/* Define to one of `_getb67', `GETB67', `getb67' for Cray-2 and Cray-YMP
   systems. This function is required for `alloca.c' support on those systems.
   */
/* #undef CRAY_STACKSEG_END */

/* Define to 1 if using `alloca.c'. */
/* #undef C_ALLOCA */

/* Define to enable work in progress code */
/* #undef ENABLE_DEVELOPING */

/* Define to enable extra checks on code */
/* #undef ENABLE_EXTRA_CHECKS */

/* Defined if --enable-krb5 used and library detected */
/* #undef ENABLE_KRB5 */

/* Define to enable MARS support */
#define ENABLE_ODBC_MARS 1

/* Define to enable ODBC wide string support */
#define ENABLE_ODBC_WIDE 1

/* Define to 1 if GNU tls use nettle as backend. */
/* #undef GNUTLS_USE_NETTLE */

/* Define to 1 if you have the `alarm' function. */
#define HAVE_ALARM 1

/* Define to 1 if you have `alloca', as a function or macro. */
#define HAVE_ALLOCA 1

/* Define to 1 if you have <alloca.h> and it should be used (not on Ultrix).
   */
#define HAVE_ALLOCA_H 1

/* Define to 1 if you have the <arpa/inet.h> header file. */
#define HAVE_ARPA_INET_H 1

/* Define to 1 if you have the `ASN1_STRING_get0_data' function. */
#define HAVE_ASN1_STRING_GET0_DATA 1

/* Define to 1 if you have the `asprintf' function. */
#define HAVE_ASPRINTF 1

/* Define to 1 if you have the `basename' function. */
#define HAVE_BASENAME 1

/* Define to 1 if you have the `BIO_get_data' function. */
#define HAVE_BIO_GET_DATA 1

/* Define if you have the clock_gettime function. */
#define HAVE_CLOCK_GETTIME 1

/* Define to 1 if you have the <com_err.h> header file. */
/* #undef HAVE_COM_ERR_H */

/* Define to 1 if you have the `daemon' function. */
#define HAVE_DAEMON 1

/* Define to 1 if you have the declaration of `cygwin_conv_path', and to 0 if
   you don't. */
/* #undef HAVE_DECL_CYGWIN_CONV_PATH */

/* Define to 1 if you have the declaration of `tzname', and to 0 if you don't.
   */
/* #undef HAVE_DECL_TZNAME */

/* Define to 1 if you have the <dirent.h> header file. */
#define HAVE_DIRENT_H 1

/* Define if you have the GNU dld library. */
/* #undef HAVE_DLD */

/* Define to 1 if you have the `dlerror' function. */
#define HAVE_DLERROR 1

/* Define to 1 if you have the <dlfcn.h> header file. */
#define HAVE_DLFCN_H 1

/* Define if you have the _dyld_func_lookup function. */
/* #undef HAVE_DYLD */

/* Define to 1 if you have the <errno.h> header file. */
#define HAVE_ERRNO_H 1

/* Define to 1 if you have the `error_message' function. */
/* #undef HAVE_ERROR_MESSAGE */

/* Define to 1 if you have the `eventfd' function. */
#define HAVE_EVENTFD 1

/* Define to 1 if you have the <fcntl.h> header file. */
#define HAVE_FCNTL_H 1

/* Define to 1 if you have the `fork' function. */
#define HAVE_FORK 1

/* Define to 1 if fseeko (and presumably ftello) exists and is declared. */
#define HAVE_FSEEKO 1

/* Define to 1 if you have the `fstat' function. */
#define HAVE_FSTAT 1

/* Define to 1 if your system provides the 5-parameter version of
   gethostbyaddr_r(). */
/* #undef HAVE_FUNC_GETHOSTBYADDR_R_5 */

/* Define to 1 if your system provides the 7-parameter version of
   gethostbyaddr_r(). */
/* #undef HAVE_FUNC_GETHOSTBYADDR_R_7 */

/* Define to 1 if your system provides the 8-parameter version of
   gethostbyaddr_r(). */
#define HAVE_FUNC_GETHOSTBYADDR_R_8 1

/* Define to 1 if your system provides the 3-parameter version of
   gethostbyname_r(). */
/* #undef HAVE_FUNC_GETHOSTBYNAME_R_3 */

/* Define to 1 if your system provides the 5-parameter version of
   gethostbyname_r(). */
/* #undef HAVE_FUNC_GETHOSTBYNAME_R_5 */

/* Define to 1 if your system provides the 6-parameter version of
   gethostbyname_r(). */
#define HAVE_FUNC_GETHOSTBYNAME_R_6 1

/* Define to 1 if your system provides the 4-parameter version of
   getpwuid_r(). */
/* #undef HAVE_FUNC_GETPWUID_R_4 */

/* Define to 1 if your system getpwuid_r() have 4 parameters and return struct
   passwd*. */
/* #undef HAVE_FUNC_GETPWUID_R_4_PW */

/* Define to 1 if your system provides the 5-parameter version of
   getpwuid_r(). */
#define HAVE_FUNC_GETPWUID_R_5 1

/* Define to 1 if your system provides the 4-parameter version of
   getservbyname_r(). */
/* #undef HAVE_FUNC_GETSERVBYNAME_R_4 */

/* Define to 1 if your system provides the 5-parameter version of
   getservbyname_r(). */
/* #undef HAVE_FUNC_GETSERVBYNAME_R_5 */

/* Define to 1 if your system provides the 6-parameter version of
   getservbyname_r(). */
#define HAVE_FUNC_GETSERVBYNAME_R_6 1

/* Define to 1 if your localtime_r return a int. */
/* #undef HAVE_FUNC_LOCALTIME_R_INT */

/* Define to 1 if your localtime_r return a struct tm*. */
#define HAVE_FUNC_LOCALTIME_R_TM 1

/* Define if you have getaddrinfo function */
#define HAVE_GETADDRINFO 1

/* Define to 1 if you have the `gethostname' function. */
#define HAVE_GETHOSTNAME 1

/* Define to 1 if you have the `gethrtime' function. */
/* #undef HAVE_GETHRTIME */

/* Define to 1 if you have the `getipnodebyaddr' function. */
/* #undef HAVE_GETIPNODEBYADDR */

/* Define to 1 if you have the `getipnodebyname' function. */
/* #undef HAVE_GETIPNODEBYNAME */

/* Define to 1 if you have the `getopt' function. */
#define HAVE_GETOPT 1

/* Define to 1 if you have the <getopt.h> header file. */
#define HAVE_GETOPT_H 1

/* Define if your getopt(3) defines and uses optreset */
/* #undef HAVE_GETOPT_OPTRESET */

/* Define to 1 if you have the `getpwuid' function. */
#define HAVE_GETPWUID 1

/* Define to 1 if you have the `getpwuid_r' function. */
#define HAVE_GETPWUID_R 1

/* Define to 1 if you have the `gettimeofday' function. */
#define HAVE_GETTIMEOFDAY 1

/* Define to 1 if you have the `getuid' function. */
#define HAVE_GETUID 1

/* Define if you have GMP library */
/* #undef HAVE_GMP */

/* Define to 1 if you have GnuTLS. */
/* #undef HAVE_GNUTLS */

/* Define to 1 if you have the <gnutls/abstract.h> header file. */
/* #undef HAVE_GNUTLS_ABSTRACT_H */

/* Define to 1 if you have the `gnutls_certificate_set_verify_function'
   function. */
/* #undef HAVE_GNUTLS_CERTIFICATE_SET_VERIFY_FUNCTION */

/* Define to 1 if you have the `gnutls_record_disable_padding' function. */
/* #undef HAVE_GNUTLS_RECORD_DISABLE_PADDING */

/* Define to 1 if you have the `gnutls_rnd' function. */
/* #undef HAVE_GNUTLS_RND */

/* Define if you have the iconv() function and it works. */
#define HAVE_ICONV 1

/* Define to 1 if you have the `inet_ntoa_r' function. */
/* #undef HAVE_INET_NTOA_R */

/* Define to 1 if you have the `inet_ntop' function. */
#define HAVE_INET_NTOP 1

/* Define to 1 if you have the <inttypes.h> header file. */
#define HAVE_INTTYPES_H 1

/* Define to 1 if you have the <iodbcinst.h> header file. */
/* #undef HAVE_IODBCINST_H */

/* Define to 1 if you have the <langinfo.h> header file. */
#define HAVE_LANGINFO_H 1

/* Define if you have the libdl library or equivalent. */
#define HAVE_LIBDL 1

/* Define if libdlloader will be built on this platform */
#define HAVE_LIBDLLOADER 1

/* Define to 1 if you have the <libgen.h> header file. */
#define HAVE_LIBGEN_H 1

/* Define to 1 if you have the <limits.h> header file. */
#define HAVE_LIMITS_H 1

/* Define to 1 if you have the <localcharset.h> header file. */
/* #undef HAVE_LOCALCHARSET_H */

/* Define to 1 if you have the `locale_charset' function. */
/* #undef HAVE_LOCALE_CHARSET */

/* Define to 1 if you have the <locale.h> header file. */
#define HAVE_LOCALE_H 1

/* Define to 1 if you have the `localtime_r' function. */
#define HAVE_LOCALTIME_R 1

/* Define to 1 if you have the <malloc.h> header file. */
#define HAVE_MALLOC_H 1

/* Define to 1 if your system provides the malloc_options variable. */
/* #undef HAVE_MALLOC_OPTIONS */

/* Define to 1 if you have the <memory.h> header file. */
#define HAVE_MEMORY_H 1

/* Define to 1 if you have the `nanosleep' function. */
#define HAVE_NANOSLEEP 1

/* Define to 1 if you have the <netdb.h> header file. */
#define HAVE_NETDB_H 1

/* Define to 1 if you have the <netinet/in.h> header file. */
#define HAVE_NETINET_IN_H 1

/* Define to 1 if you have the <netinet/tcp.h> header file. */
#define HAVE_NETINET_TCP_H 1

/* Define to 1 if nettle is present. */
/* #undef HAVE_NETTLE */

/* Define to 1 if you have the `nl_langinfo' function. */
#define HAVE_NL_LANGINFO 1

/* Define if you have the OpenSSL. */
#define HAVE_OPENSSL 1

/* Define to 1 if you have the <paths.h> header file. */
#define HAVE_PATHS_H 1

/* Define to 1 if you have the `poll' function. */
#define HAVE_POLL 1

/* Define to 1 if you have the <poll.h> header file. */
#define HAVE_POLL_H 1

/* Define if you have POSIX threads libraries and header files. */
#define HAVE_PTHREAD 1

/* Define to 1 if you have the `pthread_condattr_setclock' function. */
#define HAVE_PTHREAD_CONDATTR_SETCLOCK 1

/* Define to 1 if you have the `pthread_cond_timedwait' function. */
#define HAVE_PTHREAD_COND_TIMEDWAIT 1

/* Define to 1 if you have the `pthread_cond_timedwait_relative_np' function.
   */
/* #undef HAVE_PTHREAD_COND_TIMEDWAIT_RELATIVE_NP */

/* Define to 1 if you have the `putenv' function. */
#define HAVE_PUTENV 1

/* Define to 1 if you have the `readdir_r' function. */
#define HAVE_READDIR_R 1

/* Define to 1 if you have the GNU Readline library. */
/* #undef HAVE_READLINE */

/* Define to 1 if you have the `readpassphrase' function. */
/* #undef HAVE_READPASSPHRASE */

/* Define to 1 if you have rl_inhibit_completion. */
/* #undef HAVE_RL_INHIBIT_COMPLETION */

/* Define to 1 if you have the `rl_on_new_line' function. */
/* #undef HAVE_RL_ON_NEW_LINE */

/* Define to 1 if you have the `rl_reset_line_state' function. */
/* #undef HAVE_RL_RESET_LINE_STATE */

/* Define to 1 if you have the <roken.h> header file. */
/* #undef HAVE_ROKEN_H */

/* Define to 1 if you have the `RSA_get0_key' function. */
#define HAVE_RSA_GET0_KEY 1

/* Define to 1 if you have the `setenv' function. */
#define HAVE_SETENV 1

/* Define to 1 if you have the `setitimer' function. */
#define HAVE_SETITIMER 1

/* Define to 1 if you have the `setrlimit' function. */
#define HAVE_SETRLIMIT 1

/* Define if you have the shl_load function. */
/* #undef HAVE_SHL_LOAD */

/* Define to 1 if you have the <signal.h> header file. */
#define HAVE_SIGNAL_H 1

/* Define to 1 if you have the `socketpair' function. */
#define HAVE_SOCKETPAIR 1

/* Define to 1 if you have the SQLGetPrivateProfileString function. */
#define HAVE_SQLGETPRIVATEPROFILESTRING 1

/* Define if sqltypes.h define SQLLEN */
#define HAVE_SQLLEN 1

/* Define to 1 if the system has the type `SQLROWOFFSET'. */
/* #undef HAVE_SQLROWOFFSET */

/* Define to 1 if the system has the type `SQLROWSETSIZE'. */
/* #undef HAVE_SQLROWSETSIZE */

/* Define to 1 if the system has the type `SQLSETPOSIROW'. */
#define HAVE_SQLSETPOSIROW 1

/* Define to 1 if you have the <sql.h> header file. */
#define HAVE_SQL_H 1

/* Defined if not --disable-sspi and SSPI detected */
/* #undef HAVE_SSPI */

/* Define to 1 if you have the <stdbool.h> header file. */
#define HAVE_STDBOOL_H 1

/* Define to 1 if you have the <stddef.h> header file. */
#define HAVE_STDDEF_H 1

/* Define to 1 if you have the <stdint.h> header file. */
#define HAVE_STDINT_H 1

/* Define to 1 if you have the <stdlib.h> header file. */
#define HAVE_STDLIB_H 1

/* Define to 1 if you have the <strings.h> header file. */
#define HAVE_STRINGS_H 1

/* Define to 1 if you have the <string.h> header file. */
#define HAVE_STRING_H 1

/* Define to 1 if you have the `strlcat' function. */
/* #undef HAVE_STRLCAT */

/* Define to 1 if you have the `strlcpy' function. */
/* #undef HAVE_STRLCPY */

/* Define to 1 if you have the `strsep' function. */
#define HAVE_STRSEP 1

/* Define to 1 if you have the `strtok_r' function. */
#define HAVE_STRTOK_R 1

/* Define to 1 if you have the `strtok_s' function. */
/* #undef HAVE_STRTOK_S */

/* Define to 1 if `tm_zone' is a member of `struct tm'. */
#define HAVE_STRUCT_TM_TM_ZONE 1

/* Define to 1 if `__tm_zone' is a member of `struct tm'. */
/* #undef HAVE_STRUCT_TM___TM_ZONE */

/* Define to 1 if you have the <sys/eventfd.h> header file. */
#define HAVE_SYS_EVENTFD_H 1

/* Define to 1 if you have the <sys/ioctl.h> header file. */
#define HAVE_SYS_IOCTL_H 1

/* Define to 1 if you have the <sys/param.h> header file. */
#define HAVE_SYS_PARAM_H 1

/* Define to 1 if you have the <sys/resource.h> header file. */
#define HAVE_SYS_RESOURCE_H 1

/* Define to 1 if you have the <sys/select.h> header file. */
#define HAVE_SYS_SELECT_H 1

/* Define to 1 if you have the <sys/socket.h> header file. */
#define HAVE_SYS_SOCKET_H 1

/* Define to 1 if you have the <sys/stat.h> header file. */
#define HAVE_SYS_STAT_H 1

/* Define to 1 if you have the <sys/time.h> header file. */
#define HAVE_SYS_TIME_H 1

/* Define to 1 if you have the <sys/types.h> header file. */
#define HAVE_SYS_TYPES_H 1

/* Define to 1 if you have the <sys/wait.h> header file. */
#define HAVE_SYS_WAIT_H 1

/* Define to 1 if your `struct tm' has `tm_zone'. Deprecated, use
   `HAVE_STRUCT_TM_TM_ZONE' instead. */
#define HAVE_TM_ZONE 1

/* Define to 1 if you don't have `tm_zone' but do have the external array
   `tzname'. */
/* #undef HAVE_TZNAME */

/* Define to 1 if you have the <unistd.h> header file. */
#define HAVE_UNISTD_H 1

/* Define to 1 if you have the `usleep' function. */
#define HAVE_USLEEP 1

/* Define to 1 if you have the <valgrind/memcheck.h> header file. */
/* #undef HAVE_VALGRIND_MEMCHECK_H */

/* Define to 1 if you have the `vasprintf' function. */
#define HAVE_VASPRINTF 1

/* Define to 1 if you have the `vsnprintf' function. */
#define HAVE_VSNPRINTF 1

/* Define to 1 if you have the <wchar.h> header file. */
#define HAVE_WCHAR_H 1

/* Define to 1 if you have the <windows.h> header file. */
/* #undef HAVE_WINDOWS_H */

/* Define to 1 if you have the <winsock2.h> header file. */
/* #undef HAVE_WINSOCK2_H */

/* Define to 1 if you have the `_fseeki64' function. */
/* #undef HAVE__FSEEKI64 */

/* Define to 1 if you have the `_ftelli64' function. */
/* #undef HAVE__FTELLI64 */

/* Define to 1 if you have the `_lock_file' function. */
/* #undef HAVE__LOCK_FILE */

/* Define to 1 if you have the `_unlock_file' function. */
/* #undef HAVE__UNLOCK_FILE */

/* Define to 1 if you have the `_vscprintf' function. */
/* #undef HAVE__VSCPRINTF */

/* Define to 1 if you have the `_vsnprintf' function. */
/* #undef HAVE__VSNPRINTF */

/* Define to 1 if you have the `_xpg_accept' function. */
/* #undef HAVE__XPG_ACCEPT */

/* Define to 1 if you have the `_xpg_getpeername' function. */
/* #undef HAVE__XPG_GETPEERNAME */

/* Define to 1 if you have the `_xpg_getsockname' function. */
/* #undef HAVE__XPG_GETSOCKNAME */

/* Define to 1 if you have the `_xpg_getsockopt' function. */
/* #undef HAVE__XPG_GETSOCKOPT */

/* Define to 1 if you have the `_xpg_recvfrom' function. */
/* #undef HAVE__XPG_RECVFROM */

/* Define to 1 if you have the `__accept' function. */
/* #undef HAVE___ACCEPT */

/* Define to 1 if you have the `__getpeername' function. */
/* #undef HAVE___GETPEERNAME */

/* Define to 1 if you have the `__getsockname' function. */
/* #undef HAVE___GETSOCKNAME */

/* Define to 1 if you have the `__getsockopt' function. */
/* #undef HAVE___GETSOCKOPT */

/* Define to 1 if you have the `__recvfrom' function. */
/* #undef HAVE___RECVFROM */

/* Define as const if the declaration of iconv() needs const. */
#define ICONV_CONST

/* Define to value of INADDR_NONE if not provided by your system header files.
   */
/* #undef INADDR_NONE */

/* Define to the sub-directory where libtool stores uninstalled libraries. */
#define LT_OBJDIR ".libs/"

/* Define to 1 if the BSD-style netdb interface is reentrant. */
/* #undef NETDB_REENTRANT */

/* Define to 1 if memset(0) sets pointers to NULL. */
#define NULL_REP_IS_ZERO_BYTES 1

/* Name of package */
#define PACKAGE "freetds"

/* Define to the address where bug reports for this package should be sent. */
#define PACKAGE_BUGREPORT ""

/* Define to the full name of this package. */
#define PACKAGE_NAME "FreeTDS"

/* Define to the full name and version of this package. */
#define PACKAGE_STRING "FreeTDS 1.1.36"

/* Define to the one symbol short name of this package. */
#define PACKAGE_TARNAME "freetds"

/* Define to the home page for this package. */
#define PACKAGE_URL ""

/* Define to the version of this package. */
#define PACKAGE_VERSION "1.1.36"

/* Define to necessary symbol if this constant uses a non-standard name on
   your system. */
/* #undef PTHREAD_CREATE_JOINABLE */

/* The size of `char', as computed by sizeof. */
#define SIZEOF_CHAR 1

/* The size of `double', as computed by sizeof. */
#define SIZEOF_DOUBLE 8

/* The size of `float', as computed by sizeof. */
#define SIZEOF_FLOAT 4

/* The size of `int', as computed by sizeof. */
#define SIZEOF_INT 4

/* The size of `long', as computed by sizeof. */
#define SIZEOF_LONG 8

/* The size of `long double', as computed by sizeof. */
#define SIZEOF_LONG_DOUBLE 16

/* The size of `long long', as computed by sizeof. */
#define SIZEOF_LONG_LONG 8

/* The size of `short', as computed by sizeof. */
#define SIZEOF_SHORT 2

/* The size of `SQLWCHAR', as computed by sizeof. */
#define SIZEOF_SQLWCHAR 2

/* The size of `void *', as computed by sizeof. */
#define SIZEOF_VOID_P 8

/* The size of `wchar_t', as computed by sizeof. */
#define SIZEOF_WCHAR_T 4

/* The size of `__int64', as computed by sizeof. */
#define SIZEOF___INT64 0

/* If using the C implementation of alloca, define if you know the
   direction of stack growth for your system; otherwise it will be
   automatically deduced at runtime.
	STACK_DIRECTION > 0 => grows toward higher addresses
	STACK_DIRECTION < 0 => grows toward lower addresses
	STACK_DIRECTION = 0 => direction of growth unknown */
/* #undef STACK_DIRECTION */

/* Define to 1 if you have the ANSI C header files. */
#define STDC_HEADERS 1

/* Define to use TDS 5.0 by default */
/* #undef TDS50 */

/* Define to use TDS 7.1 by default */
/* #undef TDS71 */

/* Define to use TDS 7.2 by default */
/* #undef TDS72 */

/* Define to use TDS 7.3 by default */
/* #undef TDS73 */

/* Define to use TDS 7.4 by default */
/* #undef TDS74 */

/* Define to 1 if your compiler supports __attribute__((destructor)). */
#define TDS_ATTRIBUTE_DESTRUCTOR 1

/* define to constant to use for clock_gettime */
#define TDS_GETTIMEMILLI_CONST CLOCK_MONOTONIC

/* Define if you have pthread with mutex support */
#define TDS_HAVE_PTHREAD_MUTEX 1

/* Define if stdio support locking */
#define TDS_HAVE_STDIO_LOCKED 1

/* define to prefix format string used for 64bit integers */
#define TDS_I64_PREFIX "l"

/* Define if you don't care about thread safety */
/* #undef TDS_NO_THREADSAFE */

/* Define to 1 if last argument of SQLColAttribute it's SQLLEN * */
#define TDS_SQLCOLATTRIBUTE_SQLLEN 1

/* Define to 1 if SQLParamOptions accept SQLULEN as arguments */
#define TDS_SQLPARAMOPTIONS_SQLLEN 1

/* Defined if --enable-sybase-compat used */
/* #undef TDS_SYBASE_COMPAT */

/* Define to 1 if you can safely include both <sys/time.h> and <time.h>. */
#define TIME_WITH_SYS_TIME 1

/* Define to 1 if your <sys/time.h> declares `struct tm'. */
/* #undef TM_IN_SYS_TIME */

/* Enable extensions on AIX 3, Interix.  */
#ifndef _ALL_SOURCE
# define _ALL_SOURCE 1
#endif
/* Enable GNU extensions on systems that have them.  */
#ifndef _GNU_SOURCE
# define _GNU_SOURCE 1
#endif
/* Enable threading extensions on Solaris.  */
#ifndef _POSIX_PTHREAD_SEMANTICS
# define _POSIX_PTHREAD_SEMANTICS 1
#endif
/* Enable extensions on HP NonStop.  */
#ifndef _TANDEM_SOURCE
# define _TANDEM_SOURCE 1
#endif
/* Enable general extensions on Solaris.  */
#ifndef __EXTENSIONS__
# define __EXTENSIONS__ 1
#endif


/* Version number of package */
#define VERSION "1.1.36"

/* Define WORDS_BIGENDIAN to 1 if your processor stores words with the most
   significant byte first (like Motorola and SPARC, unlike Intel). */
#if defined AC_APPLE_UNIVERSAL_BUILD
# if defined __BIG_ENDIAN__
#  define WORDS_BIGENDIAN 1
# endif
#else
# ifndef WORDS_BIGENDIAN
/* #  undef WORDS_BIGENDIAN */
# endif
#endif

/* Enable large inode numbers on Mac OS X 10.5.  */
#ifndef _DARWIN_USE_64_BIT_INODE
# define _DARWIN_USE_64_BIT_INODE 1
#endif

/* Number of bits in a file offset, on hosts where this is settable. */
/* #undef _FILE_OFFSET_BITS */

/* Define to 1 to make fseeko visible on some hosts (e.g. glibc 2.2). */
/* #undef _LARGEFILE_SOURCE */

/* Define for large files, on AIX-style hosts. */
/* #undef _LARGE_FILES */

/* Define to 1 if on MINIX. */
/* #undef _MINIX */

/* Define to 2 if the system does not provide POSIX.1 features except with
   this defined. */
/* #undef _POSIX_1_SOURCE */

/* Define to 1 if you need to in order for `stat' and other things to work. */
/* #undef _POSIX_SOURCE */

/* Define to empty if `const' does not conform to ANSI C. */
/* #undef const */

/* Define to `__inline__' or `__inline' if that's what the C compiler
   calls it, or to nothing if 'inline' is not supported under any name.  */
#ifndef __cplusplus
/* #undef inline */
#endif

/* Define to `unsigned int' if <sys/types.h> does not define. */
/* #undef size_t */

/* type to use in place of socklen_t if not defined */
/* #undef socklen_t */
