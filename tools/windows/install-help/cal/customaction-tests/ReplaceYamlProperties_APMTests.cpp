#include "precompiled/stdafx.h"
#include "ReplaceYamlProperties.h"

TEST_F(ReplaceYamlPropertiesTests, Always_Set_apm_dd_url)
{
    value_map values = {
        {L"TRACE_DD_URL", L"https://trace.someurl.datadoghq.com"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# apm_config:

  # enabled: true

  # apm_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
apm_config:

  # enabled: true

  apm_dd_url: https://trace.someurl.datadoghq.com
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Trace_Url_Set_And_Apm_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"TRACE_DD_URL", L"https://trace.someurl.datadoghq.com"},
        {L"APM_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# apm_config:

  # enabled: true

  # apm_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
apm_config:

  enabled: false

  apm_dd_url: https://trace.someurl.datadoghq.com
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Apm_Enabled_Is_True_Correctly_Replace)
{
    value_map values = {
        {L"APM_ENABLED", L"true"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# apm_config:

  # enabled: true

  # apm_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
apm_config:

  enabled: true

  # apm_dd_url: <ENDPOINT>:<PORT>
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Apm_Enabled_Is_False_Correctly_Replace)
{
    value_map values = {
        {L"APM_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# apm_config:

  # enabled: true

  # apm_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
apm_config:

  enabled: false

  # apm_dd_url: <ENDPOINT>:<PORT>
)");
}
