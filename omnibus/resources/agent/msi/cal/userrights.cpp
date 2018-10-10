#include "stdafx.h"



PSID GetSidForUser(LPCWSTR host, LPCWSTR user) {
	SID *newsid = NULL;
	DWORD cbSid = 0;
	LPWSTR refDomain = NULL;
	DWORD cchRefDomain = 0;
	SID_NAME_USE use;
	std::string narrowdomain;
	BOOL bRet = LookupAccountName(host, user, newsid, &cbSid, refDomain, &cchRefDomain, &use);
	if (bRet) {
		// this should *never* happen, because we didn't pass in a buffer large enough for
		// the sid or the domain name.
		return NULL;
	}
	DWORD err = GetLastError();
	if (ERROR_INSUFFICIENT_BUFFER != err) {
		// we don't know what happened
		return NULL;
	}
	newsid = (SID *) new BYTE[cbSid];
	ZeroMemory(newsid, cbSid);

	refDomain = new wchar_t[cchRefDomain + 1];
	ZeroMemory(refDomain, (cchRefDomain + 1) * sizeof(wchar_t));

	// try it again
	bRet = LookupAccountName(host, user, newsid, &cbSid, refDomain, &cchRefDomain, &use);
	if (!bRet) {
		WcaLog(LOGMSG_STANDARD, "Failed to lookup account name %d", GetLastError());
		goto cleanAndFail;
	}
	if (!IsValidSid(newsid)) {
		WcaLog(LOGMSG_STANDARD, "New SID is invalid");
		goto cleanAndFail;
	}
	
	toMbcs(narrowdomain, refDomain);
	WcaLog(LOGMSG_STANDARD, "Got SID from %s", narrowdomain.c_str());
	delete[] refDomain;
	return newsid;

cleanAndFail:
	if (newsid) {
		delete[](BYTE*)newsid;
	}
	if (refDomain) {
		delete[] refDomain;
	}
	return NULL;
}
bool RemovePrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd)
{
	LSA_UNICODE_STRING lucPrivilege;
	NTSTATUS ntsResult;

	// Create an LSA_UNICODE_STRING for the privilege names.
	if (!InitLsaString(&lucPrivilege, rightToAdd))
	{
		WcaLog(LOGMSG_STANDARD, "Failed InitLsaString");
		return false;
	}

	ntsResult = LsaRemoveAccountRights(
		PolicyHandle,  // An open policy handle.
		AccountSID,    // The target SID.
		FALSE,
		&lucPrivilege, // The privileges.
		1              // Number of privileges.
	);
	if (ntsResult == 0)
	{
		WcaLog(LOGMSG_STANDARD, "Privilege removed");
		return true;
	}
	else
	{
		WcaLog(LOGMSG_STANDARD, "Privilege was not removed - %lu \n",
			LsaNtStatusToWinError(ntsResult));
	}
	return false;
}

bool AddPrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd)
{
	LSA_UNICODE_STRING lucPrivilege;
	NTSTATUS ntsResult;

	// Create an LSA_UNICODE_STRING for the privilege names.
	if (!InitLsaString(&lucPrivilege, rightToAdd))
	{
		WcaLog(LOGMSG_STANDARD, "Failed InitLsaString");
		return false;
	}

	ntsResult = LsaAddAccountRights(
		PolicyHandle,  // An open policy handle.
		AccountSID,    // The target SID.
		&lucPrivilege, // The privileges.
		1              // Number of privileges.
	);
	if (ntsResult == 0)
	{
		WcaLog(LOGMSG_STANDARD, "Privilege added");
		return true;
	}
	else
	{
		WcaLog(LOGMSG_STANDARD, "Privilege was not added - %lu \n",
			LsaNtStatusToWinError(ntsResult));
	}

	return false;
}

// returned value must be freed with LsaClose()

LSA_HANDLE GetPolicyHandle()
{
	LSA_OBJECT_ATTRIBUTES ObjectAttributes;
	NTSTATUS ntsResult;
	LSA_HANDLE lsahPolicyHandle;

	// Object attributes are reserved, so initialize to zeros.
	ZeroMemory(&ObjectAttributes, sizeof(ObjectAttributes));

	//Initialize an LSA_UNICODE_STRING to the server name.

	// Get a handle to the Policy object.
	ntsResult = LsaOpenPolicy(
		NULL, // always assume local system
		&ObjectAttributes, //Object attributes.
		POLICY_ALL_ACCESS, //Desired access permissions.
		&lsahPolicyHandle  //Receives the policy handle.
	);

	if (ntsResult != 0)
	{
		// An error occurred. Display it as a win32 error code.
		WcaLog(LOGMSG_STANDARD, "OpenPolicy returned %lu\n",
			LsaNtStatusToWinError(ntsResult));
		return NULL;
	}
	return lsahPolicyHandle;
}


bool InitLsaString(
	PLSA_UNICODE_STRING pLsaString,
	LPCWSTR pwszString
)
{
	DWORD dwLen = 0;

	if (NULL == pLsaString)
		return FALSE;

	if (NULL != pwszString)
	{
		dwLen = wcslen(pwszString);
		if (dwLen > 0x7ffe)   // String is too large
			return FALSE;
	}

	// Store the string.
	pLsaString->Buffer = (WCHAR *)pwszString;
	pLsaString->Length = (USHORT)dwLen * sizeof(WCHAR);
	pLsaString->MaximumLength = (USHORT)(dwLen + 1) * sizeof(WCHAR);

	return TRUE;
}
