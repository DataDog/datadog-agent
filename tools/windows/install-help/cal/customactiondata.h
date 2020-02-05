#pragma once
#include <map>
#include <string>

class CustomActionData
{
    public:
        CustomActionData();
        ~CustomActionData();

        bool init(MSIHANDLE hInstall);
        bool init(const std::wstring &initstring);

        bool present(const std::wstring& key) const;
        bool value(const std::wstring& key, std::wstring &val) const;
        void set_value(const std::wstring& key, const std::wstring& val);

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
        void Domain(const std::wstring &domain) {
            this->domain = domain;
        }
    private:
        MSIHANDLE hInstall;
        bool domainUser;
        std::map< std::wstring, std::wstring> values;
        std::wstring username; // qualified
        std::wstring uqusername;// unqualified
        std::wstring domain;

        bool parseUsernameData();
};
