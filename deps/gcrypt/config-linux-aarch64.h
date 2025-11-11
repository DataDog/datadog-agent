/* config.h.  Generated from config.h.in by configure.  */
/* config.h.in.  Generated from configure.ac by autoheader.  */


#ifndef _GCRYPT_CONFIG_H_INCLUDED
#define _GCRYPT_CONFIG_H_INCLUDED


/* Define if building universal (internal helper macro) */
/* #undef AC_APPLE_UNIVERSAL_BUILD */

/* Defined if --disable-asm was used to configure */
/* #undef ASM_DISABLED */

/* GIT commit id revision used to build this package */
#define BUILD_REVISION "737cc636"

/* The time this package was configured for a build */
#define BUILD_TIMESTAMP "<none>"

/* configure did not test for endianness */
/* #undef DISABLED_ENDIAN_CHECK */

/* Define if you don't want the default EGD socket name. For details see
   cipher/rndegd.c */
#define EGD_SOCKET_NAME ""

/* Enable support for Intel AES-NI instructions. */
/* #undef ENABLE_AESNI_SUPPORT */

/* Enable support for ARMv8 Crypto Extension instructions. */
#define ENABLE_ARM_CRYPTO_SUPPORT 1

/* Enable support for Intel AVX2 instructions. */
/* #undef ENABLE_AVX2_SUPPORT */

/* Enable support for Intel AVX512 instructions. */
/* #undef ENABLE_AVX512_SUPPORT */

/* Enable support for Intel AVX instructions. */
/* #undef ENABLE_AVX_SUPPORT */

/* Enable support for Intel DRNG (RDRAND instruction). */
/* #undef ENABLE_DRNG_SUPPORT */

/* Enable forcing 'soft' HW feature bits on (for testing). */
/* #undef ENABLE_FORCE_SOFT_HWFEATURES */

/* Enable support for Intel GFNI instructions. */
/* #undef ENABLE_GFNI_SUPPORT */

/* Define to support an HMAC based integrity check */
/* #undef ENABLE_HMAC_BINARY_CHECK */

/* Enable support for the jitter entropy collector. */
#define ENABLE_JENT_SUPPORT 1

/* Enable support for ARM NEON instructions. */
#define ENABLE_NEON_SUPPORT 1

/* Enable support for the PadLock engine. */
/* #undef ENABLE_PADLOCK_SUPPORT */

/* Enable support for Intel PCLMUL instructions. */
/* #undef ENABLE_PCLMUL_SUPPORT */

/* Enable support for POWER 8 (PowerISA 2.07) crypto extension. */
/* #undef ENABLE_PPC_CRYPTO_SUPPORT */

/* Enable support for Intel SHAEXT instructions. */
/* #undef ENABLE_SHAEXT_SUPPORT */

/* Enable support for Intel SSE4.1 instructions. */
/* #undef ENABLE_SSE41_SUPPORT */

/* Enable support for ARMv9 SVE2 instructions. */
/* #undef ENABLE_SVE2_SUPPORT */

/* Enable support for ARMv8 SVE instructions. */
#define ENABLE_SVE_SUPPORT 1

/* Define FIPS module version for certification */
#define FIPS_MODULE_VERSION ""

/* Define to use the GNU C visibility attribute. */
#define GCRY_USE_VISIBILITY 1

/* The default error source for libgcrypt. */
#define GPG_ERR_SOURCE_DEFAULT GPG_ERR_SOURCE_GCRYPT

/* Defined if ARM architecture is v6 or newer */
/* #undef HAVE_ARM_ARCH_V6 */

/* Define to 1 if you have the `atexit' function. */
#define HAVE_ATEXIT 1

/* Defined if the mlock() call does not work */
/* #undef HAVE_BROKEN_MLOCK */

/* Defined if compiler has '__builtin_bswap32' intrinsic */
#define HAVE_BUILTIN_BSWAP32 1

/* Defined if compiler has '__builtin_bswap64' intrinsic */
#define HAVE_BUILTIN_BSWAP64 1

/* Defined if compiler has '__builtin_clz' intrinsic */
#define HAVE_BUILTIN_CLZ 1

/* Defined if compiler has '__builtin_clzl' intrinsic */
#define HAVE_BUILTIN_CLZL 1

/* Defined if compiler has '__builtin_ctz' intrinsic */
#define HAVE_BUILTIN_CTZ 1

/* Defined if compiler has '__builtin_ctzl' intrinsic */
#define HAVE_BUILTIN_CTZL 1

/* Define to 1 if the system has the type `byte'. */
/* #undef HAVE_BYTE */

/* Defined if compiler supports clang PowerPC target attributes */
/* #undef HAVE_CLANG_ATTRIBUTE_PPC_TARGET */

/* Define to 1 if you have the `clock_gettime' function. */
#define HAVE_CLOCK_GETTIME 1

/* Defined if underlying compiler supports AArch64/NEON/crypto intrinsics */
#define HAVE_COMPATIBLE_CC_AARCH64_NEON_INTRINSICS 1

/* Defined if underlying compiler supports AArch64/NEON/crypto intrinsics with
   extra GCC flags */
#define HAVE_COMPATIBLE_CC_AARCH64_NEON_INTRINSICS_WITH_CFLAGS 1

/* Defined if underlying compiler supports PowerPC AltiVec/VSX/crypto
   intrinsics */
/* #undef HAVE_COMPATIBLE_CC_PPC_ALTIVEC */

/* Defined if underlying compiler supports PowerPC AltiVec/VSX/crypto
   intrinsics with extra GCC flags */
/* #undef HAVE_COMPATIBLE_CC_PPC_ALTIVEC_WITH_CFLAGS */

/* Defined if underlying compiler supports RISC-V vector intrinsics */
/* #undef HAVE_COMPATIBLE_CC_RISCV_VECTOR_INTRINSICS */

/* Defined if underlying compiler supports RISC-V vector intrinsics with extra
   GCC flags */
/* #undef HAVE_COMPATIBLE_CC_RISCV_VECTOR_INTRINSICS_WITH_CFLAGS */

/* Defined if underlying compiler supports x86/AVX512 intrinsics */
/* #undef HAVE_COMPATIBLE_CC_X86_AVX512_INTRINSICS */

/* Defined if underlying assembler is compatible with ARMv8/Aarch64 assembly
   implementations */
#define HAVE_COMPATIBLE_GCC_AARCH64_PLATFORM_AS 1

/* Defined if underlying assembler is compatible with amd64 assembly
   implementations */
/* #undef HAVE_COMPATIBLE_GCC_AMD64_PLATFORM_AS */

/* Defined if underlying assembler is compatible with ARM assembly
   implementations */
/* #undef HAVE_COMPATIBLE_GCC_ARM_PLATFORM_AS */

/* Defined if underlying assembler is compatible with i386 assembly
   implementations */
/* #undef HAVE_COMPATIBLE_GCC_I386_PLATFORM_AS */

/* Defined if underlying assembler is compatible with WIN32 assembly
   implementations */
/* #undef HAVE_COMPATIBLE_GCC_WIN32_PLATFORM_AS */

/* Defined if underlying assembler is compatible with WIN64 assembly
   implementations */
/* #undef HAVE_COMPATIBLE_GCC_WIN64_PLATFORM_AS */

/* Defined for Alpha platforms */
/* #undef HAVE_CPU_ARCH_ALPHA */

/* Defined for ARM AArch64 platforms */
#define HAVE_CPU_ARCH_ARM 1

/* Defined for M68k platforms */
/* #undef HAVE_CPU_ARCH_M68K */

/* Defined for MIPS platforms */
/* #undef HAVE_CPU_ARCH_MIPS */

/* Defined for PPC platforms */
/* #undef HAVE_CPU_ARCH_PPC */

/* Defined for RISC-V platforms */
/* #undef HAVE_CPU_ARCH_RISCV */

/* Defined for s390x/zSeries platforms */
/* #undef HAVE_CPU_ARCH_S390X */

/* Defined for SPARC platforms */
/* #undef HAVE_CPU_ARCH_SPARC */

/* Defined for the x86 platforms */
/* #undef HAVE_CPU_ARCH_X86 */

/* defined if the system supports a random device */
#define HAVE_DEV_RANDOM 1

/* Define to 1 if you have the <dlfcn.h> header file. */
#define HAVE_DLFCN_H 1

/* Define to 1 if you don't have `vprintf' but do have `_doprnt.' */
/* #undef HAVE_DOPRNT */

/* defined if we run on some of the PCDOS like systems (DOS, Windoze. OS/2)
   with special properties like no file modes */
/* #undef HAVE_DOSISH_SYSTEM */

/* defined if we must run on a stupid file system */
/* #undef HAVE_DRIVE_LETTERS */

/* Define to 1 if you have the `elf_aux_info' function. */
/* #undef HAVE_ELF_AUX_INFO */

/* Define to 1 if you have the `explicit_bzero' function. */
/* #undef HAVE_EXPLICIT_BZERO */

/* Define to 1 if you have the `explicit_memset' function. */
/* #undef HAVE_EXPLICIT_MEMSET */

/* Define to 1 if you have the `fcntl' function. */
#define HAVE_FCNTL 1

/* Define to 1 if you have the `flockfile' function. */
#define HAVE_FLOCKFILE 1

/* Define to 1 if you have the `ftruncate' function. */
#define HAVE_FTRUNCATE 1

/* Defined if underlying assembler supports for CFI directives */
#define HAVE_GCC_ASM_CFI_DIRECTIVES 1

/* Defined if underlying assembler supports for ELF directives */
#define HAVE_GCC_ASM_ELF_DIRECTIVES 1

/* Define if inline asm memory barrier is supported */
#define HAVE_GCC_ASM_VOLATILE_MEMORY 1

/* Defined if a GCC style "__attribute__ ((aligned (n))" is supported */
#define HAVE_GCC_ATTRIBUTE_ALIGNED 1

/* Defined if a GCC style "__attribute__ ((may_alias))" is supported */
#define HAVE_GCC_ATTRIBUTE_MAY_ALIAS 1

/* Defined if compiler supports "__attribute__ ((ms_abi))" function attribute
   */
/* #undef HAVE_GCC_ATTRIBUTE_MS_ABI */

/* Defined if compiler supports "__attribute__ ((optimize))" function
   attribute */
#define HAVE_GCC_ATTRIBUTE_OPTIMIZE 1

/* Defined if a GCC style "__attribute__ ((packed))" is supported */
#define HAVE_GCC_ATTRIBUTE_PACKED 1

/* Defined if compiler supports GCC PowerPC target attributes */
/* #undef HAVE_GCC_ATTRIBUTE_PPC_TARGET */

/* Defined if compiler supports "__attribute__ ((sysv_abi))" function
   attribute */
/* #undef HAVE_GCC_ATTRIBUTE_SYSV_ABI */

/* Defined if default calling convention is 'ms_abi' */
/* #undef HAVE_GCC_DEFAULT_ABI_IS_MS_ABI */

/* Defined if default calling convention is 'sysv_abi' */
/* #undef HAVE_GCC_DEFAULT_ABI_IS_SYSV_ABI */

/* Defined if inline assembler supports AArch32 Crypto Extension instructions
   */
/* #undef HAVE_GCC_INLINE_ASM_AARCH32_CRYPTO */

/* Defined if inline assembler supports AArch64 Crypto Extension instructions
   */
#define HAVE_GCC_INLINE_ASM_AARCH64_CRYPTO 1

/* Defined if inline assembler supports AArch64 NEON instructions */
#define HAVE_GCC_INLINE_ASM_AARCH64_NEON 1

/* Defined if inline assembler supports AArch64 SHA3/SHA512/SM3/SM4
   instructions */
/* #undef HAVE_GCC_INLINE_ASM_AARCH64_SHA3_SHA512_SM3_SM4 */

/* Defined if inline assembler supports AArch64 SVE instructions */
#define HAVE_GCC_INLINE_ASM_AARCH64_SVE 1

/* Defined if inline assembler supports AArch64 SVE2 instructions */
/* #undef HAVE_GCC_INLINE_ASM_AARCH64_SVE2 */

/* Defined if inline assembler supports AVX instructions */
/* #undef HAVE_GCC_INLINE_ASM_AVX */

/* Defined if inline assembler supports AVX2 instructions */
/* #undef HAVE_GCC_INLINE_ASM_AVX2 */

/* Defined if inline assembler supports AVX512 instructions */
/* #undef HAVE_GCC_INLINE_ASM_AVX512 */

/* Defined if inline assembler supports BMI2 instructions */
/* #undef HAVE_GCC_INLINE_ASM_BMI2 */

/* Defined if inline assembler supports GFNI instructions */
/* #undef HAVE_GCC_INLINE_ASM_GFNI */

/* Defined if inline assembler supports NEON instructions */
/* #undef HAVE_GCC_INLINE_ASM_NEON */

/* Defined if inline assembler supports PCLMUL instructions */
/* #undef HAVE_GCC_INLINE_ASM_PCLMUL */

/* Defined if inline assembler supports PowerPC AltiVec/VSX/crypto
   instructions */
/* #undef HAVE_GCC_INLINE_ASM_PPC_ALTIVEC */

/* Defined if inline assembler supports PowerISA 3.00 instructions */
/* #undef HAVE_GCC_INLINE_ASM_PPC_ARCH_3_00 */

/* Defined if inline assembler supports RISC-V instructions */
/* #undef HAVE_GCC_INLINE_ASM_RISCV */

/* Defined if inline assembler supports RISC-V vector instructions */
/* #undef HAVE_GCC_INLINE_ASM_RISCV_V */

/* Defined if inline assembler supports zSeries instructions */
/* #undef HAVE_GCC_INLINE_ASM_S390X */

/* Defined if inline assembler supports zSeries vector instructions */
/* #undef HAVE_GCC_INLINE_ASM_S390X_VX */

/* Defined if inline assembler supports SHA Extensions instructions */
/* #undef HAVE_GCC_INLINE_ASM_SHAEXT */

/* Defined if inline assembler supports SSE4.1 instructions */
/* #undef HAVE_GCC_INLINE_ASM_SSE41 */

/* Defined if inline assembler supports SSSE3 instructions */
/* #undef HAVE_GCC_INLINE_ASM_SSSE3 */

/* Defined if inline assembler supports VAES and VPCLMUL instructions */
/* #undef HAVE_GCC_INLINE_ASM_VAES_VPCLMUL */

/* Defined if compiler supports "__thread" storage class specifier */
#define HAVE_GCC_STORAGE_CLASS__THREAD 1

/* Define to 1 if you have the `getauxval' function. */
#define HAVE_GETAUXVAL 1

/* Define to 1 if you have the `getentropy' function. */
/* #undef HAVE_GETENTROPY */

/* Define to 1 if you have the `gethrtime' function. */
/* #undef HAVE_GETHRTIME */

/* Define to 1 if you have the `getpagesize' function. */
#define HAVE_GETPAGESIZE 1

/* Define to 1 if you have the `getrusage' function. */
#define HAVE_GETRUSAGE 1

/* Define to 1 if you have the `gettimeofday' function. */
#define HAVE_GETTIMEOFDAY 1

/* Defined if underlying assembler is compatible with Intel syntax assembly
   implementations */
/* #undef HAVE_INTEL_SYNTAX_PLATFORM_AS */

/* Define to 1 if you have the <inttypes.h> header file. */
#define HAVE_INTTYPES_H 1

/* Define to 1 if you have the `rt' library (-lrt). */
/* #undef HAVE_LIBRT */

/* Define to 1 if you have the `memmove' function. */
#define HAVE_MEMMOVE 1

/* Define to 1 if you have the <minix/config.h> header file. */
/* #undef HAVE_MINIX_CONFIG_H */

/* Defined if the system supports an mlock() call */
#define HAVE_MLOCK 1

/* Define to 1 if you have the `mmap' function. */
#define HAVE_MMAP 1

/* Define if we have pthread. */
#define HAVE_PTHREAD 1 

/* Define to 1 if you have the `raise' function. */
#define HAVE_RAISE 1

/* Define to 1 if you have the `rand' function. */
#define HAVE_RAND 1

/* Define to 1 if you have the <spawn.h> header file. */
/* #undef HAVE_SPAWN_H */

/* Define to 1 if you have the <stdint.h> header file. */
#define HAVE_STDINT_H 1

/* Define to 1 if you have the <stdio.h> header file. */
#define HAVE_STDIO_H 1

/* Define to 1 if you have the <stdlib.h> header file. */
#define HAVE_STDLIB_H 1

/* Define to 1 if you have the `stpcpy' function. */
#define HAVE_STPCPY 1

/* Define to 1 if you have the `strcasecmp' function. */
#define HAVE_STRCASECMP 1

/* Define to 1 if you have the `strerror' function. */
#define HAVE_STRERROR 1

/* Define to 1 if you have the `stricmp' function. */
/* #undef HAVE_STRICMP */

/* Define to 1 if you have the <strings.h> header file. */
#define HAVE_STRINGS_H 1

/* Define to 1 if you have the <string.h> header file. */
#define HAVE_STRING_H 1

/* Define to 1 if you have the `strtoul' function. */
#define HAVE_STRTOUL 1

/* Defined if compiler has '__sync_synchronize' intrinsic */
#define HAVE_SYNC_SYNCHRONIZE 1

/* Define to 1 if you have the `syscall' function. */
#define HAVE_SYSCALL 1

/* Define to 1 if you have the `sysconf' function. */
#define HAVE_SYSCONF 1

/* Define to 1 if you have the `sysctlbyname' function. */
/* #undef HAVE_SYSCTLBYNAME */

/* Define to 1 if you have the `syslog' function. */
#define HAVE_SYSLOG 1

/* Define to 1 if you have the <sys/auxv.h> header file. */
#define HAVE_SYS_AUXV_H 1

/* Define to 1 if you have the <sys/capability.h> header file. */
/* #undef HAVE_SYS_CAPABILITY_H */

/* Define to 1 if you have the <sys/mman.h> header file. */
/* #undef HAVE_SYS_MMAN_H */

/* Define to 1 if you have the <sys/random.h> header file. */
/* #undef HAVE_SYS_RANDOM_H */

/* Define to 1 if you have the <sys/stat.h> header file. */
#define HAVE_SYS_STAT_H 1

/* Define to 1 if you have the <sys/sysctl.h> header file. */
#define HAVE_SYS_SYSCTL_H 1

/* Define to 1 if you have the <sys/types.h> header file. */
#define HAVE_SYS_TYPES_H 1

/* Define to 1 if the system has the type `u16'. */
/* #undef HAVE_U16 */

/* Define to 1 if the system has the type `u32'. */
/* #undef HAVE_U32 */

/* Define to 1 if the system has the type `u64'. */
/* #undef HAVE_U64 */

/* Define to 1 if the system has the type `uintptr_t'. */
#define HAVE_UINTPTR_T 1

/* Define to 1 if you have the <unistd.h> header file. */
#define HAVE_UNISTD_H 1

/* Define to 1 if the system has the type `ushort'. */
#define HAVE_USHORT 1

/* Defined if variable length arrays are supported */
#define HAVE_VLA 1

/* Define to 1 if you have the `vprintf' function. */
#define HAVE_VPRINTF 1

/* Defined if we run on a W32 API based system */
/* #undef HAVE_W32_SYSTEM */

/* Define to 1 if you have the `wait4' function. */
#define HAVE_WAIT4 1

/* Define to 1 if you have the `waitpid' function. */
#define HAVE_WAITPID 1

/* Define to 1 if you have the <wchar.h> header file. */
#define HAVE_WCHAR_H 1

/* Defined if this is not a regular release */
/* #undef IS_DEVELOPMENT_VERSION */

/* List of available cipher algorithms */
#define LIBGCRYPT_CIPHERS "arcfour:blowfish:cast5:des:aes:twofish:serpent:rfc2268:seed:camellia:idea:salsa20:gost28147:chacha20:sm4:aria"

/* List of available digest algorithms */
#define LIBGCRYPT_DIGESTS "crc:gostr3411-94::md4:md5:rmd160:sha1:sha256:sha512:sha3:tiger:whirlpool:stribog:blake2:sm3"

/* List of available KDF algorithms */
#define LIBGCRYPT_KDFS "s2k:pkdf2:scrypt"

/* List of available public key cipher algorithms */
#define LIBGCRYPT_PUBKEY_CIPHERS "dsa:elgamal:rsa:ecc:kyber"

/* Define to the sub-directory in which libtool stores uninstalled libraries.
   */
#define LT_OBJDIR ".libs/"

/* defined to the name of the strong random device */
#define NAME_OF_DEV_RANDOM "/dev/random"

/* defined to the name of the weaker random device */
#define NAME_OF_DEV_URANDOM "/dev/urandom"

/* Name of package */
#define PACKAGE "libgcrypt"

/* Define to the address where bug reports for this package should be sent. */
#define PACKAGE_BUGREPORT "https://bugs.gnupg.org"

/* Define to the full name of this package. */
#define PACKAGE_NAME "libgcrypt"

/* Define to the full name and version of this package. */
#define PACKAGE_STRING "libgcrypt 1.11.2"

/* Define to the one symbol short name of this package. */
#define PACKAGE_TARNAME "libgcrypt"

/* Define to the home page for this package. */
#define PACKAGE_URL ""

/* Define to the version of this package. */
#define PACKAGE_VERSION "1.11.2"

/* A human readable text with the name of the OS */
#define PRINTABLE_OS_NAME "GNU/Linux"

/* The size of `uint64_t', as computed by sizeof. */
#define SIZEOF_UINT64_T 8

/* The size of `unsigned int', as computed by sizeof. */
#define SIZEOF_UNSIGNED_INT 4

/* The size of `unsigned long', as computed by sizeof. */
#define SIZEOF_UNSIGNED_LONG 8

/* The size of `unsigned long long', as computed by sizeof. */
#define SIZEOF_UNSIGNED_LONG_LONG 8

/* The size of `unsigned short', as computed by sizeof. */
#define SIZEOF_UNSIGNED_SHORT 2

/* The size of `unsigned __int128', as computed by sizeof. */
#define SIZEOF_UNSIGNED___INT128 16

/* The size of `void *', as computed by sizeof. */
#define SIZEOF_VOID_P 8

/* Define to 1 if all of the C90 standard headers exist (not just the ones
   required in a freestanding environment). This macro is provided for
   backward compatibility; new code need not use it. */
#define STDC_HEADERS 1

/* Defined if this module should be included */
#define USE_AES 1

/* Defined if this module should be included */
#define USE_ARCFOUR 1

/* Defined if this module should be included */
#define USE_ARIA 1

/* Defined if this module should be included */
#define USE_BLAKE2 1

/* Defined if this module should be included */
#define USE_BLOWFISH 1

/* Defined if this module should be included */
#define USE_CAMELLIA 1

/* define if capabilities should be used */
/* #undef USE_CAPABILITIES */

/* Defined if this module should be included */
#define USE_CAST5 1

/* Defined if this module should be included */
#define USE_CHACHA20 1

/* Defined if this module should be included */
#define USE_CRC 1

/* Defined if this module should be included */
#define USE_DES 1

/* Defined if this module should be included */
#define USE_DSA 1

/* Defined if this module should be included */
#define USE_ECC 1

/* Defined if this module should be included */
#define USE_ELGAMAL 1

/* Defined if this module should be included */
#define USE_GOST28147 1

/* Defined if this module should be included */
#define USE_GOST_R_3411_12 1

/* Defined if this module should be included */
#define USE_GOST_R_3411_94 1

/* Defined if this module should be included */
#define USE_IDEA 1

/* Defined if this module should be included */
#define USE_KYBER 1

/* Defined if this module should be included */
/* #undef USE_MD2 */

/* Defined if this module should be included */
#define USE_MD4 1

/* Defined if this module should be included */
#define USE_MD5 1

/* set this to limit filenames to the 8.3 format */
/* #undef USE_ONLY_8DOT3 */

/* defined if we use posix_spawn in test program */
/* #undef USE_POSIX_SPAWN_FOR_TESTS */

/* Defined if this module should be included */
#define USE_RFC2268 1

/* Defined if this module should be included */
#define USE_RMD160 1

/* Defined if the EGD based RNG should be used. */
/* #undef USE_RNDEGD */

/* Defined if the getentropy RNG should be used. */
/* #undef USE_RNDGETENTROPY */

/* Defined if the /dev/random RNG should be used. */
#define USE_RNDOLDLINUX 1

/* Defined if the default Unix RNG should be used. */
/* #undef USE_RNDUNIX */

/* Defined if the Windows specific RNG should be used. */
/* #undef USE_RNDW32 */

/* Defined if this module should be included */
#define USE_RSA 1

/* Defined if this module should be included */
#define USE_SALSA20 1

/* Defined if this module should be included */
#define USE_SCRYPT 1

/* Defined if this module should be included */
#define USE_SEED 1

/* Defined if this module should be included */
#define USE_SERPENT 1

/* Defined if this module should be included */
#define USE_SHA1 1

/* Defined if this module should be included */
#define USE_SHA256 1

/* Defined if this module should be included */
#define USE_SHA3 1

/* Defined if this module should be included */
#define USE_SHA512 1

/* Defined if this module should be included */
#define USE_SM3 1

/* Defined if this module should be included */
#define USE_SM4 1

/* Enable extensions on AIX 3, Interix.  */
#ifndef _ALL_SOURCE
# define _ALL_SOURCE 1
#endif
/* Enable general extensions on macOS.  */
#ifndef _DARWIN_C_SOURCE
# define _DARWIN_C_SOURCE 1
#endif
/* Enable general extensions on Solaris.  */
#ifndef __EXTENSIONS__
# define __EXTENSIONS__ 1
#endif
/* Enable GNU extensions on systems that have them.  */
#ifndef _GNU_SOURCE
# define _GNU_SOURCE 1
#endif
/* Enable X/Open compliant socket functions that do not require linking
   with -lxnet on HP-UX 11.11.  */
#ifndef _HPUX_ALT_XOPEN_SOCKET_API
# define _HPUX_ALT_XOPEN_SOCKET_API 1
#endif
/* Identify the host operating system as Minix.
   This macro does not affect the system headers' behavior.
   A future release of Autoconf may stop defining this macro.  */
#ifndef _MINIX
/* # undef _MINIX */
#endif
/* Enable general extensions on NetBSD.
   Enable NetBSD compatibility extensions on Minix.  */
#ifndef _NETBSD_SOURCE
# define _NETBSD_SOURCE 1
#endif
/* Enable OpenBSD compatibility extensions on NetBSD.
   Oddly enough, this does nothing on OpenBSD.  */
#ifndef _OPENBSD_SOURCE
# define _OPENBSD_SOURCE 1
#endif
/* Define to 1 if needed for POSIX-compatible behavior.  */
#ifndef _POSIX_SOURCE
/* # undef _POSIX_SOURCE */
#endif
/* Define to 2 if needed for POSIX-compatible behavior.  */
#ifndef _POSIX_1_SOURCE
/* # undef _POSIX_1_SOURCE */
#endif
/* Enable POSIX-compatible threading on Solaris.  */
#ifndef _POSIX_PTHREAD_SEMANTICS
# define _POSIX_PTHREAD_SEMANTICS 1
#endif
/* Enable extensions specified by ISO/IEC TS 18661-5:2014.  */
#ifndef __STDC_WANT_IEC_60559_ATTRIBS_EXT__
# define __STDC_WANT_IEC_60559_ATTRIBS_EXT__ 1
#endif
/* Enable extensions specified by ISO/IEC TS 18661-1:2014.  */
#ifndef __STDC_WANT_IEC_60559_BFP_EXT__
# define __STDC_WANT_IEC_60559_BFP_EXT__ 1
#endif
/* Enable extensions specified by ISO/IEC TS 18661-2:2015.  */
#ifndef __STDC_WANT_IEC_60559_DFP_EXT__
# define __STDC_WANT_IEC_60559_DFP_EXT__ 1
#endif
/* Enable extensions specified by ISO/IEC TS 18661-4:2015.  */
#ifndef __STDC_WANT_IEC_60559_FUNCS_EXT__
# define __STDC_WANT_IEC_60559_FUNCS_EXT__ 1
#endif
/* Enable extensions specified by ISO/IEC TS 18661-3:2015.  */
#ifndef __STDC_WANT_IEC_60559_TYPES_EXT__
# define __STDC_WANT_IEC_60559_TYPES_EXT__ 1
#endif
/* Enable extensions specified by ISO/IEC TR 24731-2:2010.  */
#ifndef __STDC_WANT_LIB_EXT2__
# define __STDC_WANT_LIB_EXT2__ 1
#endif
/* Enable extensions specified by ISO/IEC 24747:2009.  */
#ifndef __STDC_WANT_MATH_SPEC_FUNCS__
# define __STDC_WANT_MATH_SPEC_FUNCS__ 1
#endif
/* Enable extensions on HP NonStop.  */
#ifndef _TANDEM_SOURCE
# define _TANDEM_SOURCE 1
#endif
/* Enable X/Open extensions.  Define to 500 only if necessary
   to make mbstate_t available.  */
#ifndef _XOPEN_SOURCE
/* # undef _XOPEN_SOURCE */
#endif


/* Defined if this module should be included */
#define USE_TIGER 1

/* Defined if this module should be included */
#define USE_TWOFISH 1

/* Defined if this module should be included */
#define USE_WHIRLPOOL 1

/* Version number of package */
#define VERSION "1.11.2"

/* Defined if compiled symbols have a leading underscore */
/* #undef WITH_SYMBOL_UNDERSCORE */

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

/* Expose all libc features (__DARWIN_C_FULL). */
#define _DARWIN_C_SOURCE 1

/* To allow the use of Libgcrypt in multithreaded programs we have to use
    special features from the library. */
#ifndef _REENTRANT
# define _REENTRANT 1
#endif


/* Define to supported assembler block keyword, if plain 'asm' was not
   supported */
/* #undef asm */

/* Define to empty if `const' does not conform to ANSI C. */
/* #undef const */

/* Define to `__inline__' or `__inline' if that's what the C compiler
   calls it, or to nothing if 'inline' is not supported under any name.  */
#ifndef __cplusplus
/* #undef inline */
#endif

/* Define as a signed integer type capable of holding a process identifier. */
/* #undef pid_t */

/* Define to `unsigned int' if <sys/types.h> does not define. */
/* #undef size_t */

/* Define to the type of an unsigned integer type wide enough to hold a
   pointer, if such a type exists, and if the system does not define it. */
/* #undef uintptr_t */


#define _GCRYPT_IN_LIBGCRYPT 1

/* Add .note.gnu.property section for Intel CET in assembler sources
   when CET is enabled.  */
#if defined(__ASSEMBLER__) && defined(__CET__)
# include <cet.h>
#endif

/* If the configure check for endianness has been disabled, get it from
   OS macros.  This is intended for making fat binary builds on OS X.  */
#ifdef DISABLED_ENDIAN_CHECK
# if defined(__BIG_ENDIAN__)
#  define WORDS_BIGENDIAN 1
# elif defined(__LITTLE_ENDIAN__)
/* #  undef WORDS_BIGENDIAN */
# else
#  error "No endianness found"
# endif
#endif /*DISABLED_ENDIAN_CHECK*/

/* We basically use the original Camellia source.  Make sure the symbols
   properly prefixed.  */
#define CAMELLIA_EXT_SYM_PREFIX _gcry_

#endif /*_GCRYPT_CONFIG_H_INCLUDED*/

