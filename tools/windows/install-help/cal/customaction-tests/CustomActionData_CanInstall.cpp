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
DDAGENTUSER_NAME=different_domain\\test
DDAGENTUSER_PASSWORD=1234
)");
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
