#pragma once

#include "targetver.h"

#define WIN32_LEAN_AND_MEAN             // Exclude rarely-used stuff from Windows headers
// Windows Header Files:
#include <windows.h>
#include <ntsecapi.h>
#include <AccCtrl.h>
#include <AclAPI.h>
#include <sddl.h>
#include <shlwapi.h>

#include <stdlib.h>
#include <strsafe.h>
#include <msiquery.h>
#include <lmaccess.h>
#include <lmerr.h>

// std c++ lib
#include <string>
#include <map>
#include <sstream>

// WiX Header Files:
#include <wcautil.h>


// TODO: reference additional headers your program requires here
#include "winacl.h"
#include "customaction.h"
#include "strings.h"


