#include "stdafx.h"
#include "gtest/gtest.h"
#include "PropertyReplacer.h"

class UpdateYamlConfigTests : public testing::Test
{
  protected:
};

/*
     values = {
        {L"APIKEY", L"1234567890"},
        {L"SITE", L"datadoghq.eu"},
        {L"HOSTNAME", L"raspberrypi"},
        {L"LOGS_ENABLED", L"true"},
        {L"CMD_PORT", L"8000"},
        {L"DD_URL", L"http://example.org:8999"},
        {L"LOGS_DD_URL", L"https://logs.example.org"},
        {L"TRACE_DD_URL", L"https://trace.example.org:5858"},
        {L"PROCESS_DD_URL", L"https://proc.example.org"},
        {L"PROCESS_ENABLED", L"true"},
        {L"APM_ENABLED", L"true"},
        {L"TAGS", L"region=eastus2,aws_org=2"},
        {L"PROXY_HOST", L"172.14.0.1"},
        {L"PROXY_PORT", L"4242"},
        {L"PROXY_USER", L"pUser"},
        {L"PROXY_PASSWORD", L"pPass"},
        {L"HOSTNAME_FQDN_ENABLED", L"true"}
    };
 */

typedef std::map<std::wstring, std::wstring> value_map;
property_retriever propertyRetriever(value_map const &values)
{
    return [values](std::wstring const &propertyName) -> std::optional<std::wstring>
    {
        auto it = values.find(propertyName);
        if (it != values.end())
        {
            return it->second;
        }
        return std::nullopt;
    };
}

TEST_F(UpdateYamlConfigTests, When_APIKEY_Present_Replace_It)
{
    value_map values = {{L"APIKEY", L"1234567890"}};
    std::wstring result = replace_yaml_properties(
LR"(
## @param api_key - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key:)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result,
LR"(
## @param api_key - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key: 1234567890)");
}

TEST_F(UpdateYamlConfigTests, When_Optional_Proxy_Values_Present_Dont_Do_Anything)
{
    value_map values =
    {
        {L"PROXY_PORT", L"4242"},
        {L"PROXY_USER", L"pUser"},
        {L"PROXY_PASSWORD", L"pPass"}
    };
    std::wstring result = replace_yaml_properties(LR"(
## @param proxy - custom object - optional
## If you need a proxy to connect to the Internet, provide it here (default:
## disabled). Refer to https://docs.datadoghq.com/agent/proxy/ to understand how to use these settings.
## For Logs proxy information, refer to https://docs.datadoghq.com/agent/proxy/#proxy-for-logs
#
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
## @param proxy - custom object - optional
## If you need a proxy to connect to the Internet, provide it here (default:
## disabled). Refer to https://docs.datadoghq.com/agent/proxy/ to understand how to use these settings.
## For Logs proxy information, refer to https://docs.datadoghq.com/agent/proxy/#proxy-for-logs
#
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)");
}

TEST_F(UpdateYamlConfigTests, When_PROXY_HOST_Present_Replace_It)
{
    value_map values =
        {{L"PROXY_HOST", L"172.14.0.1"},
         {L"PROXY_PORT", L"4242"},
         {L"PROXY_USER", L"pUser"},
         {L"PROXY_PASSWORD", L"pPass"}};
    std::wstring result = replace_yaml_properties(LR"(
## @param proxy - custom object - optional
## If you need a proxy to connect to the Internet, provide it here (default:
## disabled). Refer to https://docs.datadoghq.com/agent/proxy/ to understand how to use these settings.
## For Logs proxy information, refer to https://docs.datadoghq.com/agent/proxy/#proxy-for-logs
#
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
## @param proxy - custom object - optional
## If you need a proxy to connect to the Internet, provide it here (default:
## disabled). Refer to https://docs.datadoghq.com/agent/proxy/ to understand how to use these settings.
## For Logs proxy information, refer to https://docs.datadoghq.com/agent/proxy/#proxy-for-logs
#
proxy:
  https: pUser:pPass@172.14.0.1:4242
  http: pUser:pPass@172.14.0.1:4242

#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)");
}
