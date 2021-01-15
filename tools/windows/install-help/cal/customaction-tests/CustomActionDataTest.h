#pragma once

#include "stdafx.h"
#include "gtest/gtest.h"

class CustomActionDataTest : public testing::Test
{
  protected:
    void SetUp() override
    {
        propertyDDAgentUserName = L"DDAGENTUSER_NAME";
        propertyDDAgentUserPassword = L"DDAGENTUSER_PASSWORD";
    }
};
