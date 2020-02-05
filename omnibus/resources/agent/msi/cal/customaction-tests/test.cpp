#include "stdafx.h"
#include "gtest/gtest.h"

// Case 2
TEST(CanInstallTest_OnDomainController, When_ServiceDoesNotExists_And_UserExists_WithPassword_ReturnsTrue) {
    CustomActionData customActionCtx;
    propertyDDAgentUserPassword = L"pass";
    customActionCtx.set_value(propertyDDAgentUserPassword, L"1234");
    bool shouldResetPass;

    bool result = canInstall(true, false, false, customActionCtx, shouldResetPass);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST(CanInstallTest_OnDomainController, When_ServiceExists_And_NoUser_ReturnsFalse) {
    CustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(true, false, true, customActionCtx, shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST(CanInstallTest_OnDomainController, When_ServiceDoesNotExists_And_UserExists_ButNoPassword_ReturnsFalse) {
    CustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(true, false, false, customActionCtx, shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST(CanInstallTest_OnDomainController, When_ServiceExists_And_UserDoesNotExists_WithUserInDifferentDomain_ReturnsFalse) {
    CustomActionData customActionCtx;
    domainname = L"domain";
    customActionCtx.Domain(L"different_domain");
    bool shouldResetPass;

    bool result = canInstall(true, false, true, customActionCtx, shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}
