#include "precompiled/stdafx.h"
#include "ReplaceYamlProperties.h"

TEST_F(ReplaceYamlPropertiesTests, When_Optional_Proxy_Values_Present_Dont_Do_Anything)
{
    value_map values = {{L"PROXY_PORT", L"4242"}, {L"PROXY_USER", L"pUser"}, {L"PROXY_PASSWORD", L"pPass"}};
    std::wstring result = replace_yaml_properties(
        LR"(
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)",
        propertyRetriever(values));

    EXPECT_EQ(result,
              LR"(
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)");
}

TEST_F(ReplaceYamlPropertiesTests, When_PROXY_HOST_Present_Replace_It)
{
    value_map values = {{L"PROXY_HOST", L"172.14.0.1"},
                        {L"PROXY_PORT", L"4242"},
                        {L"PROXY_USER", L"pUser"},
                        {L"PROXY_PASSWORD", L"pPass"}};
    std::wstring result = replace_yaml_properties(
        LR"(
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)",
        propertyRetriever(values));

    EXPECT_EQ(result,
              LR"(
proxy:
  https: http://pUser:pPass@172.14.0.1:4242
  http: http://pUser:pPass@172.14.0.1:4242

#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)");
}

TEST_F(ReplaceYamlPropertiesTests, Respect_PROXY_HOST_Scheme)
{
    value_map values = {{L"PROXY_HOST", L"ftps://mydomain.org"},
                        {L"PROXY_PORT", L"4242"},
                        {L"PROXY_USER", L"pUser"},
                        {L"PROXY_PASSWORD", L"pPass"}};
    std::wstring result = replace_yaml_properties(
        LR"(
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)",
        propertyRetriever(values));

    EXPECT_EQ(result,
              LR"(
proxy:
  https: ftps://pUser:pPass@mydomain.org:4242
  http: ftps://pUser:pPass@mydomain.org:4242

#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>)");
}
