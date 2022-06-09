#include "stdafx.h"
#include "SID.h"

SidPtr WellKnownSid::Create(WELL_KNOWN_SID_TYPE sidType)
{
    DWORD sidLength = 0;
    if (!CreateWellKnownSid(sidType, nullptr, nullptr, &sidLength))
    {
        auto error = GetLastError();
        if (error == ERROR_INVALID_PARAMETER || error == ERROR_INSUFFICIENT_BUFFER)
        {
            auto ntAuthorityPsId = MakeSid(sidLength);
            if (CreateWellKnownSid(sidType, nullptr, ntAuthorityPsId.get(), &sidLength))
            {
                return ntAuthorityPsId;
            }
        }
    }
    throw std::exception("could not create well known sid");
}
