#include <gtest/gtest.h>
#include "customaction-tests.h"
#include "customaction.h"

class ShouldUpdateConfigTest : public testing::Test
{

};

TEST_F(ShouldUpdateConfigTest, When_APIKEY_At_Start_Of_Line_Do_NOT_Replace)
{
    const std::wstring inputConfig =
        L"\n"
        L"#########################\n"
        L"## Basic Configuration ##\n"
        L"#########################\n"
        L"\n"
        L"api_key: asd\n"
        L"\n"
        L"## @param site - string - optional - default: datadoghq.com\n"
        L"## The site of the Datadog intake to send Agent data to.\n"
        L"## Set to 'datadoghq.eu' to send data to the EU site.\n"
        L"#\n"
        L"# site: datadoghq.com\n"
        L"\n";
    EXPECT_FALSE(ShouldUpdateConfig(inputConfig));
}

TEST_F(ShouldUpdateConfigTest, When_APIKEY_NOT_At_Start_Of_Line_Do_Replace)
{
    const std::wstring inputConfig = L"\n"
                                     L"#########################\n"
                                     L"## Basic Configuration ##\n"
                                     L"#########################\n"
                                     L"\n"
                                     L"  api_key: asd\n"
                                     L"\n"
                                     L"## @param site - string - optional - default: datadoghq.com\n"
                                     L"## The site of the Datadog intake to send Agent data to.\n"
                                     L"## Set to 'datadoghq.eu' to send data to the EU site.\n"
                                     L"#\n"
                                     L"# site: datadoghq.com\n"
                                     L"\n";
    EXPECT_TRUE(ShouldUpdateConfig(inputConfig));
}
