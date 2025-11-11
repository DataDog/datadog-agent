#pragma once
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

    typedef struct {
        const char* display_name;
        const char* version;
        const char* install_date;
        const char* source;
        uint8_t     is_64bit;
        const char* publisher;
        const char* status;
        const char* product_code;
    } MSStoreEntry;

    __declspec(dllexport)
        int ListStoreEntries(MSStoreEntry** out_array, int32_t* out_count);

    __declspec(dllexport)
        void FreeStoreEntries(MSStoreEntry* entries, int32_t count);

#ifdef __cplusplus
}
#endif
