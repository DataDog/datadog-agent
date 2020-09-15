// Tips for Getting Started: 
//   1. Use the Solution Explorer window to add/manage files
//   2. Use the Team Explorer window to connect to source control
//   3. Use the Output window to see build output and other messages
//   4. Use the Error List window to view errors
//   5. Go to Project > Add New Item to create new code files, or Project > Add Existing Item to add existing code files to the project
//   6. In the future, to open this project again, go to File > Open > Project and select the .sln file

#ifndef PCH_H
#define PCH_H

// TODO: add headers that you want to pre-compile here
#include "..\cal\targetver.h"

#define NTDDI_VERSION NTDDI_VISTA
#define WIN32_LEAN_AND_MEAN             // Exclude rarely-used stuff from Windows headers
// Windows Header Files:
#include <windows.h>
#include <bcrypt.h>
#include <ntsecapi.h>
#include <AccCtrl.h>
#include <AclAPI.h>
#include <sddl.h>
#include <shlwapi.h>
#include <shlobj.h>
#include <UserEnv.h>

#include <stdlib.h>
#include <strsafe.h>
#include <msiquery.h>
#include <lm.h>
#include <lmaccess.h>
#include <lmerr.h>

// std c++ lib
#include <string>
#include <sstream>
#include <map>
#include <sstream>
#include <filesystem>

// WiX Header Files:
#include "uninstall-cmd.h"
#include "cmdargs.h"


// TODO: reference additional headers your program requires here
#include "..\cal\winacl.h"
#include "..\cal\customactiondata.h"
#include "..\cal\customaction.h"
#include "..\cal\strings.h"
#include "..\cal\ddreg.h"


#include "resource.h"

#ifdef _WIN64
// define __REGISTER_ALL_SERVICES to have the custom action install APM & process
// agent.  Otherwise, only the core service will be installed.
#define __REGISTER_ALL_SERVICES
#endif
#endif //PCH_H
