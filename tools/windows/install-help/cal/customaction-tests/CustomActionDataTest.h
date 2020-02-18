#pragma once

#include "gtest/gtest.h"
#include "stdafx.h"

class CustomActionDataTest : public testing::Test
{
protected:
    void SetUp() override
    {
        propertyDDAgentUserName = L"DDAGENTUSER_NAME";
        propertyDDAgentUserPassword = L"DDAGENTUSER_PASSWORD";
    }
};
