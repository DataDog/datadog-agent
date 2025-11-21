#pragma once
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    char *display_name;
    char *version;
    char *install_date;
    uint8_t is_64bit;
    char *publisher;
    char *product_code;
} MSStoreEntry;

__declspec(dllexport) int ListStoreEntries(MSStoreEntry **out_array, int32_t *out_count);

__declspec(dllexport) void FreeStoreEntries(MSStoreEntry *entries, int32_t count);

#ifdef __cplusplus
}
#endif