
#define RC_FILE_VERSION MAJ_VER,MIN_VER,PATCH_VER,0

#define STRINGIFY(x) #x
#define TO_STRING(x) STRINGIFY(x)

#define FILE_VERSION_STRING TO_STRING(MAJ_VER.MIN_VER.PATCH_VER.0)

#define IDD_DIALOG1                     101
#define IDC_EDIT1                       1001
#define IDC_TICKET_EDIT                 1001
#define IDC_EMAIL_EDIT                  1002