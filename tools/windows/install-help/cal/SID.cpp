#include "stdafx.h"
#include "SID.h"

std::optional<sid_ptr> WellKnownSID::NTAuthority()
{
    SID_IDENTIFIER_AUTHORITY sidIdAuthority = SECURITY_NT_AUTHORITY;
    sid_ptr sid = make_sid(GetSidLengthRequired(1));
    if (InitializeSid(sid.get(), &sidIdAuthority, 1))
    {
        return sid;
    }
    return std::nullopt;
}
