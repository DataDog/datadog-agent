#ifndef __KCONFIG_H
#define __KCONFIG_H

#include <linux/kconfig.h>

// undefine this arm64 assembly config option because the __MTE_PREAMBLE define is defined poorly
// and causes compilation errors on 5.9+
#ifdef CONFIG_ARM64_MTE
#undef CONFIG_ARM64_MTE
#endif

#endif
