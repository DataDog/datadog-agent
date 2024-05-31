/**
 * This header file is included in kconfig.h and ktypes.h, which should make
 * this metadata available to all object files.
 */

#ifndef __BPF_METADATA_H__
#define __BPF_METADATA_H__

#if defined(__x86_64__) || defined(__TARGET_ARCH_x86)
char __dd_metadata_arch[] __attribute__((section("dd_metadata"), used)) = "<arch:x86_64>";
#elif defined(__aarch64__) || defined(__TARGET_ARCH_arm64)
char __dd_metadata_arch[] __attribute__((section("dd_metadata"), used)) = "<arch:arm64>";
#else
char __dd_metadata_arch[] __attribute__((section("dd_metadata"), used)) = "<arch:unset>";
#endif

#endif
