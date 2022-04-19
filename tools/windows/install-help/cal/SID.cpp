#include "stdafx.h"
#include "SID.h"

std::optional<sid_ptr> WellKnownSID::LocalSystem()
{
    SID_IDENTIFIER_AUTHORITY sidIdAuthority = SECURITY_NT_AUTHORITY;
    PSID sid;
    if (AllocateAndInitializeSid(&sidIdAuthority, 1, SECURITY_LOCAL_SYSTEM_RID, 0, 0, 0, 0, 0, 0, 0, &sid))
    {
        return std::optional(sid_ptr(static_cast<SID *>(sid)));
    }
    return std::nullopt;
}

std::optional<sid_ptr> WellKnownSID::NTAuthority()
{
    SID_IDENTIFIER_AUTHORITY sidIdAuthority = SECURITY_NT_AUTHORITY;
    PSID sid;
    if (AllocateAndInitializeSid(&sidIdAuthority, 1, 0, 0, 0, 0, 0, 0, 0, 0, &sid))
    {
        return std::optional(sid_ptr(static_cast<SID *>(sid)));
    }
    return std::nullopt;
}
