// ReSharper disable StringLiteralTypo
#include "stdafx.h"
#include "CustomActionDataTest.h"

TEST_F(CustomActionDataTest, With_DomainUser_Parse_Correctly)
{
    CustomActionData customActionCtx;
    customActionCtx.init(LR"(
    DDAGENTUSER_NAME=TEST\username;
)");
    EXPECT_EQ(customActionCtx.Username(), L"TEST\\username");
    EXPECT_EQ(customActionCtx.UnqualifiedUsername(), L"username");
    EXPECT_EQ(customActionCtx.Domain(), L"TEST");
    EXPECT_TRUE(customActionCtx.isUserDomainUser());
    EXPECT_FALSE(customActionCtx.isUserLocalUser());
}

void expect_string_equal(CustomActionData const &customActionData, std::wstring const &prop, std::wstring const &expected)
{
    std::wstring val;
    customActionData.value(prop, val);
    EXPECT_STREQ(val.c_str(), expected.c_str());
}

TEST_F(CustomActionDataTest, With_SingleEmptyProperty_Parse_Correctly)
{
    CustomActionData customActionCtx;
    customActionCtx.init(LR"(
        TEST_PROPERTY=;
)");
    expect_string_equal(customActionCtx, L"TEST_PROPERTY", L"");
}

TEST_F(CustomActionDataTest, With_SinglePropertyWithSpacea_Parse_Correctly)
{
    CustomActionData customActionCtx;
    customActionCtx.init(LR"(
        PROP_WITH_SPACE=    ;
)");
    expect_string_equal(customActionCtx, L"PROP_WITH_SPACE", L"");
}

TEST_F(CustomActionDataTest, With_ManyEmptyProperties_Parse_Correctly)
{
    CustomActionData customActionCtx;
    customActionCtx.init(LR"(
        PROXY_HOST=;
        PROXY_PORT=;
        PROXY_USER=;
)");
    expect_string_equal(customActionCtx, L"PROXY_HOST", L"");
    expect_string_equal(customActionCtx, L"PROXY_PORT", L"");
    expect_string_equal(customActionCtx, L"PROXY_USER", L"");
}
