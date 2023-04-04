#pragma once

#include <gtest/gtest.h>

#include <PropertyView.h>

class TestPropertyView : public StaticPropertyView
{
  public:
    TestPropertyView::TestPropertyView(std::wstring &data)
    {
        parseKeyValueString(data, this->values);
    }
};
