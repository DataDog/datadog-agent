#pragma once
#include <heapapi.h>
#include <memory>
#include <optional>

template <class P> struct heap_deleter
{
    typedef P *pointer;

    void operator()(pointer ptr) const
    {
        HeapFree(GetProcessHeap(), 0, ptr);
    }
};
typedef std::unique_ptr<SID, heap_deleter<SID>> sid_ptr;

inline sid_ptr make_sid(size_t sidLength)
{
    return sid_ptr(static_cast<sid_ptr::pointer>(HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, sidLength)));
}

namespace WellKnownSID
{
std::optional<sid_ptr> NTAuthority();
} // namespace WellKnownSID
