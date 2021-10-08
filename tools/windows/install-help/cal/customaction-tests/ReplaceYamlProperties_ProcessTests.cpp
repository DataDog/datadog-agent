#include "stdafx.h"
#include "ReplaceYamlProperties.h"

TEST_F(ReplaceYamlPropertiesTests, When_Apm_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_ENABLED", L"true"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:

  enabled: "true"
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Apm_Disabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:

  enabled: "disabled"
)");
}

TEST_F(ReplaceYamlPropertiesTests, Always_Set_process_dd_url)
{
    value_map values = {
        {L"PROCESS_DD_URL", L"https://process.someurl.datadoghq.com"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result,
              LR"(
process_config:
  process_dd_url: https://process.someurl.datadoghq.com

  # enabled: "true"
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Process_Url_Set_And_Apm_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_DD_URL", L"https://process.someurl.datadoghq.com"},
        {L"PROCESS_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result,
              LR"(
process_config:
  process_dd_url: https://process.someurl.datadoghq.com

  enabled: "disabled"
)");
}
