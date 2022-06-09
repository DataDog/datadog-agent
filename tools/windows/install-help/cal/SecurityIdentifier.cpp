#include "stdafx.h"
#include <vector>
#include <sstream>
#include <sddl.h>
#include "SecurityIdentifier.h"

SecurityIdentifier::SecurityIdentifier(SidPtr &sid)
    : _psid(std::move(sid))
{
    LookupNameFromSid(L"");
}

SecurityIdentifier::SecurityIdentifier(std::wstring const& accountName, std::wstring const& systemName)
{
    DWORD cbSid = 0;
    DWORD cchRefDomain = 0;
    SID_NAME_USE use;

    LookupAccountName(systemName.c_str(), accountName.c_str(), nullptr, &cbSid, nullptr, &cchRefDomain, &use);
    _psid = MakeSid(cbSid);
    if (_psid == nullptr)
    {
        throw std::exception("cannot allocate memory for SID");
    }

    std::vector<wchar_t> refDomain;
    // +1 in case cchRefDomain == 0
    refDomain.resize(cchRefDomain + 1);
    if (!LookupAccountName(systemName.c_str(), accountName.c_str(), _psid.get(), &cbSid, &refDomain[0], &cchRefDomain, &use))
    {
        throw std::exception("cannot lookup account name");
    }

    if (!IsValidSid(_psid.get()))
    {
        throw std::exception("sid is invalid");
    }

    LookupNameFromSid(systemName);
}

SecurityIdentifier::SecurityIdentifier(SecurityIdentifier &&other) noexcept
	: _psid(std::move(other._psid))
	, _name(std::move(other._name))
	, _domain(std::move(other._domain))
{
}

SecurityIdentifier& SecurityIdentifier::operator=(SecurityIdentifier &&other) noexcept
{
    _psid = std::move(other._psid);
    _name = std::move(other._name);
    _domain = std::move(other._domain);
    return *this;
}

std::wstring const& SecurityIdentifier::GetName() const
{
    return _name;
}

std::wstring const& SecurityIdentifier::GetDomain() const
{
    return _domain;
}

std::wstring SecurityIdentifier::ToString() const
{
    LPWSTR sidStr;
    if (ConvertSidToStringSid(_psid.get(), &sidStr) != 0)
    {
        std::wstringstream result;
        result << _domain << "\\" << _name << L" (" << sidStr << L")";
        LocalFree(sidStr);
        return result.str();
    }
    throw std::exception("could not convert SID to string");
}

SID * SecurityIdentifier::GetSid() const
{
    return _psid.get();
}

SecurityIdentifier::~SecurityIdentifier() = default;

SecurityIdentifier SecurityIdentifier::CreateWellKnown(WELL_KNOWN_SID_TYPE sidType)
{
    DWORD sidLength = 0;
    if (!CreateWellKnownSid(sidType, nullptr, nullptr, &sidLength))
    {
        DWORD error = GetLastError();
        if (error == ERROR_INVALID_PARAMETER || error == ERROR_INSUFFICIENT_BUFFER)
        {
            auto ntAuthorityPsId = MakeSid(sidLength);
            if (CreateWellKnownSid(sidType, nullptr, ntAuthorityPsId.get(), &sidLength))
            {
                return SecurityIdentifier(ntAuthorityPsId);
            }
        }
    }
    Win32Exception::ThrowFromLastError();
}

SecurityIdentifier SecurityIdentifier::FromString(std::wstring sidStr)
{
    PSID sid;
    if (ConvertStringSidToSid(sidStr.c_str(), &sid))
    {
        auto sidPtr = SidPtr(static_cast<SidPtr::pointer>(sid));
        return SecurityIdentifier(sidPtr);
    }
    Win32Exception::ThrowFromLastError();
}

void SecurityIdentifier::LookupNameFromSid(std::wstring const& systemName)
{
    DWORD cchName = 0;
    DWORD cchRefDomain = 0;
    SID_NAME_USE use;
    if (LookupAccountSid(systemName.c_str(), _psid.get(), nullptr, &cchName, nullptr, &cchRefDomain, &use) != 0)
    {
        // this should *never* happen, because we didn't pass in a buffer large enough for
        // the sid or the domain name.
        throw std::exception("unexpected result while requesting size of buffers for LookupAccountSid");
    }
    DWORD error = GetLastError();
    if (ERROR_INSUFFICIENT_BUFFER != error)
    {
        // we don't know what happened
        throw std::exception("unexpected error");
    }

    std::vector<wchar_t> name, domain;
    name.resize(cchName);
    domain.resize(cchRefDomain);

    // try it again
    if (LookupAccountSid(systemName.c_str(), _psid.get(), &name[0], &cchName, &domain[0], &cchRefDomain, &use) == 0)
    {
        throw std::exception("cannot lookup account SID");
    }

    _name = std::wstring(&name[0]);
    _domain = std::wstring(&domain[0]);
}

bool SecurityIdentifier::IsWellKnown(WELL_KNOWN_SID_TYPE sidType) const
{
    return IsWellKnownSid(_psid.get(), sidType);
}

bool SecurityIdentifier::PrefixEqual(const SecurityIdentifier& other) const
{
    return EqualPrefixSid(_psid.get(), other._psid.get());
}

bool SecurityIdentifier::DomainEqual(const SecurityIdentifier& other) const
{
    BOOL domainEqual = FALSE;
    BOOL areDomainOrBuiltinSids = EqualDomainSid(_psid.get(), other._psid.get(), &domainEqual);
    return areDomainOrBuiltinSids && domainEqual;
}

bool SecurityIdentifier::operator==(const SecurityIdentifier& other) const
{
    return EqualSid(_psid.get(), other._psid.get());
}
