rule StealerDLL_Amadey {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-07-28"
        description = "Detects Amadey's Stealer DLL"
        yarahub_uuid = "a39bd717-10a6-4851-b916-87decfd9d167"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d4944b1c2a2636220b189ab9b8dbbc00"
        malpedia_family = "win.amadey"
    strings:
        $StealerDLL_pdb = "D:\\Mktmp\\StealerDLL\\x64\\Release\\STEALERDLL.pdb"
        $StealerDLL_dll = "STEALERDLL.dll"
        $powershell = "powershell -Command Compress-Archive -Path"
    condition:
        uint16(0) == 0x5a4d and
	($StealerDLL_pdb or 
	$StealerDLL_dll) and 
	$powershell
}