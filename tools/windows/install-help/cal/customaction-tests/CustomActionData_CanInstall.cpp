#include "customaction-tests.h"

#include "CustomActionDataTest.h"
#include "customactiondata.h"
#include "TargetMachineMock.h"
#include "customaction.h"

// Case 2
TEST_F(CustomActionDataTest, When_ServiceDoesNotExists_And_UserExists_WithPassword_ReturnsTrue) {
    auto tm = std::make_shared<TargetMachineMock>();
    ON_CALL(*tm, Detect).WillByDefault(testing::Return(ERROR_SUCCESS));
    CustomActionData customActionCtx(tm);

    customActionCtx.init(LR"(
DDAGENTUSER_NAME=different_domain\test
DDAGENTUSER_PASSWORD=1234
)");

    // Since "different_domain\test" is a bogus domain\user,
    // we allocate a LOCAL_SID that is guaranteed to not match NT_AUTHORITY
    SID_IDENTIFIER_AUTHORITY sidIdAuthority = SECURITY_LOCAL_SID_AUTHORITY;
    PSID sid = nullptr;
    EXPECT_TRUE(AllocateAndInitializeSid(&sidIdAuthority, 1, 0, 0, 0, 0, 0, 0, 0, 0, &sid));
    sid_ptr sid_ptr(static_cast<SID *>(sid));
    // and set it on the CustomActionData so that the test has a valid SID to use.
    customActionCtx.Sid(sid_ptr);
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
    auto tm = std::make_shared<TargetMachineMock>();
    ON_CALL(*tm, Detect).WillByDefault(testing::Return(ERROR_SUCCESS));
    CustomActionData customActionCtx(tm);
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
    auto tm = std::make_shared<TargetMachineMock>();
    ON_CALL(*tm, Detect).WillByDefault(testing::Return(ERROR_SUCCESS));
    CustomActionData customActionCtx(tm);
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
    auto tm = std::make_shared<TargetMachineMock>();
    ON_CALL(*tm, Detect).WillByDefault(testing::Return(ERROR_SUCCESS));
    CustomActionData customActionCtx(tm);
    customActionCtx.init(L"DDAGENTUSER_NAME=different_domain\\test");
    //ddAgentUserDomain = L"domain";

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
    auto tm = std::make_shared<TargetMachineMock>();
    ON_CALL(*tm, Detect).WillByDefault(testing::Return(ERROR_SUCCESS));
    CustomActionData customActionCtx(tm);
    customActionCtx.init(LR"(
DDAGENTUSER_NAME=TEST.LOCAL\username
DDAGENTUSER_PASSWORD=pass
)");
    //ddAgentUserDomain = L"domain";

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

TEST_F(CustomActionDataTest, When_User_Is_NTAUTHORITY_Dont_Reset_Password)
{
    auto tm = std::make_shared<TargetMachineMock>();
    ON_CALL(*tm, Detect).WillByDefault(testing::Return(ERROR_SUCCESS));
    CustomActionData customActionCtx(tm);
    customActionCtx.init(LR"(
DDAGENTUSER_NAME=NT AUTHORITY\SYSTEM
)");

    bool shouldResetPass;

    bool result =
        canInstall(true /*isDC*/, true /*ddUserExists*/, false /*ddServiceExists*/, customActionCtx, shouldResetPass);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}
