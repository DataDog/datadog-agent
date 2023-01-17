#include "customaction-tests.h"

#include "CustomActionDataTest.h"
#include "customactiondata.h"
#include "TargetMachineMock.h"
#include "customaction.h"

// Case 2
TEST_F(CustomActionDataTest, When_ServiceDoesNotExists_And_UserExists_WithPassword_ReturnsTrue)
{
    bool shouldResetPass;

    bool result = canInstall(true,  /*isDC*/
                             false, /*isReadOnlyDC*/
                             true,  /*ddUserExists*/
                             false, /*isServiceAccount*/
                             false, /*isNtAuthority*/
                             true,  /*isUserDomainUser*/
                             true,  /*haveUserPassword*/
                             L"",   /*userDomain*/
                             L"",   /*computerDomain*/
                             false, /*ddServiceExists*/
                             shouldResetPass, NULL);
    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_ServiceExists_And_NoUser_ReturnsFalse)
{
    bool shouldResetPass;

    bool result = canInstall(true,  /*isDC*/
                             false, /*isReadOnlyDC*/
                             false, /*ddUserExists*/
                             false, /*isServiceAccount*/
                             false, /*isNtAuthority*/
                             false, /*isUserDomainUser*/
                             false, /*haveUserPassword*/
                             L"",   /*userDomain*/
                             L"",   /*computerDomain*/
                             true,  /*ddServiceExists*/
                             shouldResetPass, NULL);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_ServiceExists_And_UserDoesNotExists_WithUserInDifferentDomain_ReturnsFalse)
{
    bool shouldResetPass;

    bool result = canInstall(true,  /*isDC*/
                             false, /*isReadOnlyDC*/
                             false, /*ddUserExists*/
                             false, /*isServiceAccount*/
                             false, /*isNtAuthority*/
                             true,  /*isUserDomainUser*/
                             false, /*haveUserPassword*/
                             L"a",  /*userDomain*/
                             L"b",  /*computerDomain*/
                             true,  /*ddServiceExists*/
                             shouldResetPass, NULL);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_ServiceDoesNotExists_And_UserDoesNotExists_WithUserInSameDomain_ReturnsTrue)
{
    bool shouldResetPass;

    bool result = canInstall(true,  /*isDC*/
                             false, /*isReadOnlyDC*/
                             false, /*ddUserExists*/
                             false, /*isServiceAccount*/
                             false, /*isNtAuthority*/
                             true,  /*isUserDomainUser*/
                             true,  /*haveUserPassword*/
                             L"a",  /*userDomain*/
                             L"a",  /*computerDomain*/
                             false, /*ddServiceExists*/
                             shouldResetPass, NULL);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}

TEST_F(CustomActionDataTest, When_User_Is_NTAUTHORITY_Dont_Reset_Password)
{
    bool shouldResetPass;

    bool result = canInstall(true,  /*isDC*/
                             false, /*isReadOnlyDC*/
                             true,  /*ddUserExists*/
                             false, /*isServiceAccount*/
                             true,  /*isNtAuthority*/
                             false, /*isUserDomainUser*/
                             false, /*haveUserPassword*/
                             L"",   /*userDomain*/
                             L"",   /*computerDomain*/
                             false, /*ddServiceExists*/
                             shouldResetPass, NULL);

    EXPECT_TRUE(result);
    EXPECT_FALSE(shouldResetPass);
}
