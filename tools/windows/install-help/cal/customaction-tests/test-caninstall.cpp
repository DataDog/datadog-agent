#include "stdafx.h"
#include "CustomActionDataTest.h"

// Case 2
TEST_F(CustomActionDataTest, When_ServiceDoesNotExists_And_UserExists_WithPassword_ReturnsTrue) {
    CustomActionData customActionCtx;
    customActionCtx.init(L"DDAGENTUSER_NAME=different_domain\\test;DDAGENTUSER_PASSWORD=1234");
    bool shouldResetPass;

    bool result = canInstall(
        true    /*isDC*/,
        true    /*ddUserExists*/,
        false   /*ddServiceExists*/,
        customActionCtx,
        shouldResetPass);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_ServiceExists_And_NoUser_ReturnsFalse) {
    CustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(
        true    /*isDC*/,
        false   /*ddUserExists*/,
        true    /*ddServiceExists*/,
        customActionCtx,
        shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_ServiceDoesNotExists_And_UserExists_ButNoPassword_ReturnsFalse) {
    CustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(
        true    /*isDC*/,
        false   /*ddUserExists*/,
        false   /*ddServiceExists*/,
        customActionCtx,
        shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_ServiceExists_And_UserDoesNotExists_WithUserInDifferentDomain_ReturnsFalse) {
    CustomActionData customActionCtx;
    customActionCtx.init(L"DDAGENTUSER_NAME=different_domain\\test");
    domainname = L"domain";

    bool shouldResetPass;

    bool result = canInstall(
        true    /*isDC*/,
        false   /*ddUserExists*/,
        true    /*ddServiceExists*/,
        customActionCtx,
        shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}


TEST_F(CustomActionDataTest, When_ServiceDoesNotExists_And_UserDoesNotExists_WithUserInDotLocalDomain_ReturnsTrue) {
    CustomActionData customActionCtx;
    customActionCtx.init(L"DDAGENTUSER_NAME=TEST.LOCAL\\username");
    domainname = L"test";

    bool shouldResetPass;

    bool result = canInstall(
        true    /*isDC*/,
        false   /*ddUserExists*/,
        false    /*ddServiceExists*/,
        customActionCtx,
        shouldResetPass);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}
