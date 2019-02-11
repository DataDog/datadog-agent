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

        const std::wstring& getUsername() const{
            return this->username;
        }
        const std::wstring& getUserdomain() const{
            return this->userdomain;
        }
        const std::wstring& getFullUsername() const{
            return this->fullusername;
        }
        const wchar_t* getDomainPtr() const{
            return this->domainPtr;
        }
        const wchar_t* getUserPtr() const{
            return this->userPtr;
        }
        const std::string& getFullUsernameMbcs() const {
            return this->fullusermbcs;
        }
        const std::wstring& getQualifiedUsername() const {
            return this->qualifieduser;
        }
    private:
        MSIHANDLE hInstall;
        std::map< std::wstring, std::wstring> values;
        std::wstring username; // unqualified
        std::wstring userdomain;
        std::wstring fullusername; // userdomain\username
        std::string fullusermbcs;
        // if it's a local account, it's just the username.
        // otherwise, the full name.
        std::wstring qualifieduser;
        const wchar_t * domainPtr;
        const wchar_t * userPtr;

        bool parseUsernameData();
};
