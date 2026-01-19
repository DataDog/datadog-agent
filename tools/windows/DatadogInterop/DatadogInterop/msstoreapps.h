#pragma once
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif
typedef struct MSStoreEntry {
    const wchar_t *display_name;

    uint16_t version_major;
    uint16_t version_minor;
    uint16_t version_build;
    uint16_t version_revision;

    int64_t install_date; // unix timestamp (seconds since epoch)
    uint64_t is_64bit; // uint64 to avoid padding

    const wchar_t *publisher;
    const wchar_t *product_code;
} MSStoreEntry;

typedef struct MSStore {
    int64_t count; // int64 to avoid padding
    MSStoreEntry *entries;
} MSStore;

__declspec(dllexport) BOOL GetStore(MSStore **out);

__declspec(dllexport) BOOL FreeStore(MSStore *store);

#ifdef __cplusplus
}
struct MSStoreInternal : MSStore {
    std::vector<MSStoreEntry> entriesVec;
    std::vector<winrt::hstring> strings;
};
#endif
