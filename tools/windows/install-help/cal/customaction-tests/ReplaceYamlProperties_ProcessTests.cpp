#include "precompiled/stdafx.h"
#include "ReplaceYamlProperties.h"

TEST_F(ReplaceYamlPropertiesTests, When_Process_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_ENABLED", L"true"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # process_collection:
    # enabled: false

  # container_collection:
    # enabled: true

  ## Deprecated - use `process_collection.enabled` and `container_collection.enabled` instead
  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:

  process_collection:
    enabled: true

  # container_collection:
    # enabled: true

  ## Deprecated - use `process_collection.enabled` and `container_collection.enabled` instead
  # enabled: "true"
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Process_Disabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:
  # process_collection:
    # enabled: false
  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:
  process_collection:
    enabled: false
  # enabled: "true"
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

    EXPECT_EQ(result, LR"(
process_config:
  process_dd_url: https://process.someurl.datadoghq.com

  # enabled: "true"
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Process_Url_Set_And_Process_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_DD_URL", L"https://process.someurl.datadoghq.com"},
        {L"PROCESS_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:
  # process_collection:
    # enabled: false
  # enabled: "true"
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:
  process_dd_url: https://process.someurl.datadoghq.com
  process_collection:
    enabled: false
  # enabled: "true"
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Process_Discovery_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_DISCOVERY_ENABLED", L"true"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # enabled: "disabled"

  # process_discovery:
    # enabled: false
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:

  # enabled: "disabled"

  process_discovery:
    enabled: true
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Process_Url_Set_And_Process_Discovery_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"PROCESS_DD_URL", L"https://process.someurl.datadoghq.com"},
        {L"PROCESS_DISCOVERY_ENABLED", L"true"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# process_config:

  # process_discovery:
    # enabled: false
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
process_config:
  process_dd_url: https://process.someurl.datadoghq.com

  process_discovery:
    enabled: true
)");
}
