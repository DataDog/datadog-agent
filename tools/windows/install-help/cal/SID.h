#pragma once
#include <WinBase.h>
#include <memory>
#include <string>

template <class P> struct LocalDeleter
{
    typedef P *Pointer;

    void operator()(Pointer ptr) const
    {
        LocalFree(ptr);
    }
};
typedef std::unique_ptr<SID, LocalDeleter<SID>> SidPtr;

inline SidPtr MakeSid(size_t sidLength)
{
    return SidPtr(static_cast<SidPtr::pointer>(LocalAlloc(LPTR, sidLength)));
}

namespace WellKnownSid
{
SidPtr Create(WELL_KNOWN_SID_TYPE sidType);
std::wstring ToString(WELL_KNOWN_SID_TYPE sidType);
} // namespace WellKnownSid
