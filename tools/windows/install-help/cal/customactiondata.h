#pragma once
#include <map>
#include <string>
#include "TargetMachine.h"

class CustomActionData
{
    public:
        CustomActionData();
        ~CustomActionData();

        bool init(MSIHANDLE hInstall);
        bool init(const std::wstring &initstring);

        bool present(const std::wstring& key) const;
        bool value(const std::wstring& key, std::wstring &val) const;

        bool isUserDomainUser() const;

        bool isUserLocalUser() const;

        const std::wstring& UnqualifiedUsername() const;

        const std::wstring& Username() const;

        const std::wstring& Domain() const;
        void Domain(const std::wstring& domain);

        bool installSysprobe() const;

        const TargetMachine& GetTargetMachine() const;
    private:
        MSIHANDLE hInstall;
        TargetMachine machine;
        bool domainUser;
        std::map< std::wstring, std::wstring> values;
        std::wstring _unqualifiedUsername;
        std::wstring _domain;
        std::wstring _fqUsernameFromCli;
        bool doInstallSysprobe;

        bool parseUsernameData();
        bool parseSysprobeData();
};
