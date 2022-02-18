#include <gtest/gtest.h>
#include "customaction-tests.h"
#include "customaction.h"
#include "customactiondata.h"
#undef min
#undef max
#include <optional>

class InstallInfoTest : public testing::Test
{
};

int getRandomUiLevel(int minLevel, int maxLevel = std::numeric_limits<int>::max())
{
    // We don't need true random numbers
    std::srand(std::time(nullptr)); // NOLINT(cert-msc51-cpp, clang-diagnostic-shorten-64-to-32)
    return std::min(std::max(std::rand(), minLevel), maxLevel);
}

bool writeInstallInfo(const CustomActionData &customActionData);

TEST_F(InstallInfoTest, When_UILevel_NotSpecified_Install_Fails)
{
    CustomActionData data;
    data.init(L"");
    EXPECT_FALSE(writeInstallInfo(data));
}

TEST_F(InstallInfoTest, When_UILevel_Specified_Doesnt_Fail_Install)
{
    CustomActionData data;
    data.init(L"UILevel=2");
    EXPECT_TRUE(writeInstallInfo(data));
}

TEST_F(InstallInfoTest, When_UILevel_NotSpecified_But_With_Override_Doesnt_Fail_Install)
{
    CustomActionData data;
    data.init(L"OVERRIDE_INSTALLATION_METHOD=test");
    EXPECT_TRUE(writeInstallInfo(data));
}

TEST_F(InstallInfoTest, When_UILevel_And_Override_Specified_Doesnt_Fail_Install)
{
    CustomActionData data;
    data.init(L"UILevel=42\r\nOVERRIDE_INSTALLATION_METHOD=test");
    EXPECT_TRUE(writeInstallInfo(data));
}

std::optional<std::wstring> GetInstallMethod(const CustomActionData &customActionData);

TEST_F(InstallInfoTest, When_UILevel_NotSpecified_GetInstallMethod_Returns_Empty)
{
    CustomActionData data;
    data.init(L"");
    const auto installMethod = GetInstallMethod(data);
    EXPECT_FALSE(installMethod.has_value());
}

TEST_F(InstallInfoTest, When_UILevel_Less_Or_Eq_2_GetInstallMethod_Returns_Quiet)
{
    CustomActionData data;
    const int uiLevel = getRandomUiLevel(0, 2);
    std::wstringstream params;
    params << L"UILevel=" << uiLevel;
    data.init(params.str());
    auto installMethod = GetInstallMethod(data);
    EXPECT_TRUE(installMethod.has_value());
    EXPECT_EQ(installMethod.value(), L"windows_msi_quiet");
}

TEST_F(InstallInfoTest, When_UILevel_Greater_Than_2_GetInstallMethod_Returns_Gui)
{
    CustomActionData data;
    const int uiLevel = getRandomUiLevel(3);
    std::wstringstream params;
    params << L"UILevel=" << uiLevel;
    data.init(params.str());
    auto installMethod = GetInstallMethod(data);
    EXPECT_TRUE(installMethod.has_value());
    EXPECT_EQ(installMethod.value(), L"windows_msi_gui");
}

TEST_F(InstallInfoTest, When_UILevel_And_Override_Specified_GetInstallMethod_Returns_Override)
{
    CustomActionData data;
    const int uiLevel = getRandomUiLevel(0);
    std::wstringstream params;
    params << L"UILevel=" << uiLevel << L"\r\nOVERRIDE_INSTALLATION_METHOD=test";
    data.init(params.str());
    auto installMethod = GetInstallMethod(data);
    EXPECT_TRUE(installMethod.has_value());
    EXPECT_EQ(installMethod.value(), L"test");
}
