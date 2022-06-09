#pragma once
#include <string>
#include "SID.h"

/// <summary>
/// SecurityIdentifier is a wrapper class for an SID.
/// </summary>
/// It should be used whenever possible instead of manipulating an SID/SidPtr directly.
class SecurityIdentifier
{
public:
	SecurityIdentifier(std::wstring const& accountName, std::wstring const& systemName = L"");
	SecurityIdentifier(SecurityIdentifier&&) noexcept;
    SecurityIdentifier& operator=(SecurityIdentifier &&) noexcept;
	SecurityIdentifier(SecurityIdentifier const&) = delete;
	SecurityIdentifier& operator=(SecurityIdentifier const&) = delete;
	~SecurityIdentifier();

	static SecurityIdentifier CreateWellKnown(WELL_KNOWN_SID_TYPE sidType);
    static SecurityIdentifier FromString(std::wstring sidStr);

	[[nodiscard]] std::wstring const& GetName() const;
	[[nodiscard]] std::wstring const& GetDomain() const;
	[[nodiscard]] std::wstring ToString()  const;

    /**
     * \brief Returns the underlying SID
     * Only use it when needed for Win32 API calls, do not store.
     * \return 
     */
    [[nodiscard]] SID *GetSid() const;

    [[nodiscard]] bool IsWellKnown(WELL_KNOWN_SID_TYPE sidType) const;
    [[nodiscard]] bool PrefixEqual(const SecurityIdentifier& other) const;
    [[nodiscard]] bool DomainEqual(const SecurityIdentifier& other) const;
    [[nodiscard]] bool operator==(const SecurityIdentifier& other) const;

private:
    SidPtr _psid;
    std::wstring _name;
    std::wstring _domain;

	explicit SecurityIdentifier(SidPtr &sid);
    void LookupNameFromSid(std::wstring const& systemName);
};
