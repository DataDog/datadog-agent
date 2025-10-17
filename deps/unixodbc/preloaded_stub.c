/* Stub implementation for libltdl preloaded symbols
 * This provides the symbol that ltdl.c expects when HAVE_LIBDLLOADER is defined.
 * Since we're not preloading any modules, we provide an empty list.
 * 
 * The symbol name expands from:
 *   LT_CONC3(lt_, LTDLOPEN, _LTX_preloaded_symbols)
 * where LTDLOPEN=libltdl, resulting in:
 *   lt_libltdl_LTX_preloaded_symbols
 *
 * libtool generates a file that contains this symbol list 
 * during the build. So we need to have something similar here.
 */

#include "libltdl/ltdl.h"

/* The preloaded symbols list - empty since we use dynamic loading */
#if defined(__GNUC__)
__attribute__((weak))
#endif
const lt_dlsymlist lt_libltdl_LTX_preloaded_symbols[] = {
    { 0, 0 }
};

