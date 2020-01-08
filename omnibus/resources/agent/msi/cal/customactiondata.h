#pragma once
#include <map>
#include <string>
class CustomActionData
{
    public:
        CustomActionData();
        ~CustomActionData();

        bool init(MSIHANDLE hInstall);

        bool present(const std::wstring& key) const;
        bool value( std::wstring& key, std::wstring &val) ;

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
        const std::wstring& Hostname() const {
            return this->hostname;
        }
    private:
        MSIHANDLE hInstall;
        bool domainUser;
        std::map< std::wstring, std::wstring> values;
        std::wstring username; // qualified
        std::wstring uqusername;// unqualified
        std::wstring domain;
        std::wstring hostname;


        bool parseUsernameData();
};
