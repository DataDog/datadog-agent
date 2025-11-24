#include "msstore_internal.h"

extern "C" __declspec(dllexport) int MSStore_ListEntries(MSStore **out) {
    return ListStoreEntries(reinterpret_cast<MSStoreInternal **>(out));
}

extern "C" __declspec(dllexport) void MSStore_FreeEntries(MSStore *store) {
    FreeStoreEntries(reinterpret_cast<MSStoreInternal *>(store));
}
