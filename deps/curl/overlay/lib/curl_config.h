// This is not generated. As we add configurations make this bigger.
// Having this indirection allows us to avoid an action to copy the
// os specific config.h to the canonical name befor building the library.

#ifdef __APPLE__
    #include "TargetConditionals.h"
    #if TARGET_OS_MAC
        #include "darwin_arm64/lib/curl_config.h"
    #else
        // Unsupported platform
        #error "Unsupported Apple platform"
    #endif
#elif defined(__linux__)
    #include "linux_arm64/lib/curl_config.h"
#else
#error "no curl_config.h for this platform"
#endif
