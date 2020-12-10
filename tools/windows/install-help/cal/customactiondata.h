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
        bool UserParamMismatch() const {
            return userParamMismatch;
        }
        const TargetMachine& GetTargetMachine() const {
            return machine;
        }
    private:
        MSIHANDLE hInstall;
        TargetMachine machine;
        bool domainUser;
        bool userParamMismatch;
        std::map< std::wstring, std::wstring> values;
        std::wstring username; // qualified
        std::wstring uqusername;// unqualified
        std::wstring domain;
        bool doInstallSysprobe;

        std::wstring pvsUser;       // previously installed user, read from registry
        std::wstring pvsDomain;     // previously installed domain for user, read from registry

        bool findPreviousUserInfo();
        void checkForUserMismatch(bool previousInstall, bool userSupplied, std::wstring &computed_domain, std::wstring &computed_user);
        void findSuppliedUserInfo(std::wstring &input, std::wstring &computed_domain, std::wstring &computed_user);


        bool parseUsernameData();
        bool parseSysprobeData();
};
