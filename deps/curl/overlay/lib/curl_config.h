// This is not generated. As we add configurations make this bigger

#ifdef __APPLE__
    #include "TargetConditionals.h"
    #if TARGET_OS_MAC
        #include "darwin_arm64/lib/curl_config.h"
    #else
        // Unsupported platform
        #error "Unsupported Apple platform"
    #endif
#else
#error "no curl_config.h for this platform"
#endif
