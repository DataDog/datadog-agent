#include "stdafx.h"

class FakeCustomActionData : public ICustomActionData
{
public:
    ~FakeCustomActionData() override {}

    bool present(const std::wstring& key) const override
    {
        return this->values.count(key) != 0 ? true : false;
    }

    bool value(std::wstring& key, std::wstring& val) override {
        if (values.count(key) == 0) {
            return false;
        }
        val = values[key];
        return true;
    }

    bool isUserDomainUser() const override { return domainUser; }
    bool isUserLocalUser() const override { return !isUserDomainUser(); }
    const std::wstring& Username() const override { return username;  }
    const std::wstring& UnqualifiedUsername() const override { return uqusername; }
    const std::wstring& Domain() const override { return domain; }
    const std::wstring& Hostname() const override { return hostname; }

    bool domainUser;
    std::map< std::wstring, std::wstring> values;
    std::wstring username; // qualified
    std::wstring uqusername;// unqualified
    std::wstring domain;
    std::wstring hostname;
};

TEST(CanInstallTest_OnDomainController, When_ServiceExists_And_NoUser_ReturnsFalse) {
    FakeCustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(true, false, true, customActionCtx, shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST(CanInstallTest_OnDomainController, When_ServiceDoesNotExists_And_UserExists_ButNoPassword_ReturnsFalse) {
    FakeCustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(true, false, false, customActionCtx, shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}


TEST(CanInstallTest_OnDomainController, When_ServiceDoesNotExists_And_UserExists_WithPassword_ReturnsTrue) {
    FakeCustomActionData customActionCtx;
    propertyDDAgentUserPassword = L"pass";
    customActionCtx.values[propertyDDAgentUserPassword] = L"1234";
    bool shouldResetPass;

    bool result = canInstall(true, false, false, customActionCtx, shouldResetPass);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}
