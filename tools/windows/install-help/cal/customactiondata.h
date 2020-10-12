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

        bool isUserDomainUser() const {
            return domainUser;
        }
        bool isUserLocalUser() const {
            return !domainUser;
        }

        const std::wstring& Username() const {
            return this->username;
        }
        const std::wstring& UnqualifiedUsername() const {
            return this->uqusername;
        }
        const std::wstring& Domain() const {
            return this->domain;
        }

        bool installSysprobe() const {
            return doInstallSysprobe;
        }

        const TargetMachine& GetTargetMachine() const {
            return machine;
        }
    private:
        MSIHANDLE hInstall;
        TargetMachine machine;
        bool domainUser;
        std::map< std::wstring, std::wstring> values;
        std::wstring username; // qualified
        std::wstring uqusername;// unqualified
        std::wstring domain;
        bool doInstallSysprobe;

        bool parseUsernameData();
        bool parseSysprobeData();
};
