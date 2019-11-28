#pragma once
#include <map>
#include <string>

#include "import_export.h"

class CA_API ICustomActionData
{
public:
    virtual ~ICustomActionData() {};
    virtual bool present(const std::wstring& key) const = 0;
    virtual bool value(std::wstring& key, std::wstring& val) = 0;
    virtual bool isUserDomainUser() const = 0;
    virtual bool isUserLocalUser() const = 0;
    virtual const std::wstring& Username() const = 0;
    virtual const std::wstring& UnqualifiedUsername() const = 0;
    virtual const std::wstring& Domain() const = 0;
    virtual const std::wstring& Hostname() const = 0;
};

class CA_API CustomActionData : public ICustomActionData
{
    public:
        CustomActionData();
        ~CustomActionData();

        bool init(MSIHANDLE hInstall);
        bool init(const std::wstring &initstring);

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
    private:
        MSIHANDLE hInstall;
        bool domainUser;
        std::map< std::wstring, std::wstring> values;
        std::wstring username; // qualified
        std::wstring uqusername;// unqualified
        std::wstring domain;

        bool parseUsernameData();
};
