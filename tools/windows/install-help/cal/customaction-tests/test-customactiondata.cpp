// ReSharper disable StringLiteralTypo
#include "stdafx.h"
#include "CustomActionDataTest.h"

TEST_F(CustomActionDataTest, Handle_Username) {
    CustomActionData customActionCtx;
    customActionCtx.init(L"DDAGENTUSER_NAME=TEST\\username");
    EXPECT_EQ(customActionCtx.Username(), L"TEST\\username");
    EXPECT_EQ(customActionCtx.UnqualifiedUsername(), L"username");
    EXPECT_EQ(customActionCtx.Domain(), L"TEST");
    EXPECT_TRUE(customActionCtx.isUserDomainUser());
    EXPECT_FALSE(customActionCtx.isUserLocalUser());
}
