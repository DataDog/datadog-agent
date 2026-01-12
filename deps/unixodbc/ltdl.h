// dl_wrapper.h - Drop-in replacement for ltdl.h on Linux
// We are not using libltdl, so we redefine the functions here
// to ensure that dynamic loading works. Since we also don't
// have any cross-platform code here, we can just call
// the kernel functions directly.
#ifndef DL_WRAPPER_H
#define DL_WRAPPER_H

/* stddef.h was transitively included somewhere within libltdl 
 * and used all over the place. Since we don't have libltdl, 
 * we need to include it here to avoid build errors.
 */
#include <stddef.h>
#include <dlfcn.h>

// Type definitions
typedef void* lt_dlhandle;

// Function mappings
#define lt_dlinit()                 (0)  // Always succeeds, returns 0
#define lt_dlexit()                 (0)  // Always succeeds, returns 0
#define lt_dlopen(path)             dlopen((path), RTLD_NOW | RTLD_GLOBAL)
#define lt_dlsym(handle, symbol)    dlsym((handle), (symbol))
#define lt_dlerror()                dlerror()
#define lt_dlclose(handle)          dlclose(handle)

// Constants (if needed)
#define LT_DLSYM_CONST              /* empty */

#endif // DL_WRAPPER_H