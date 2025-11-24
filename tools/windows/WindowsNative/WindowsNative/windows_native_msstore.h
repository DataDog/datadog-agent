#pragma once
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// --------- MS Store API (module) ----------

typedef struct MSStoreEntry {
    const wchar_t *display_name;

    uint16_t version_major;
    uint16_t version_minor;
    uint16_t version_build;
    uint16_t version_revision;

    int64_t install_date; // unix ms
    uint8_t is_64bit;

    const wchar_t *publisher;
    const wchar_t *product_code;
} MSStoreEntry;

typedef struct MSStore {
    int32_t count;
    MSStoreEntry *entries;
} MSStore;

__declspec(dllexport) int MSStore_ListEntries(MSStore **out);

__declspec(dllexport) void MSStore_FreeEntries(MSStore *store);

#ifdef __cplusplus
}
#endif
