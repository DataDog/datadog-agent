#pragma once
class ddRegKey
{
public:
    ddRegKey();
    ~ddRegKey();

    bool getStringValue(const wchar_t* valname, std::wstring& val);

private:
    HKEY hKeyRoot;

};
