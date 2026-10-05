rule Win_Clipboard_Clipper_Thengavar {
    meta:
        description = "Detects malware manipulating the Windows clipboard for clipping or crypto stealing attacks"
        author = "Thengavar"
        date = "2026-09-23"
        confidence = "high"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_license = "CC0 1.0"
	yarahub_uuid = "ec698240-465a-4306-a6f9-bbf31280566f"
	yarahub_reference_md5 = "09f452f2e024c65e4fd5c90733509530"

    strings:
        $api1 = "GlobalAlloc" ascii wide
        $api2 = "GlobalLock" ascii wide
        $api3 = "GlobalUnlock" ascii wide
        
        $clip1 = "EmptyClipboard" ascii wide
        $clip2 = "SetClipboardData" ascii wide

        $pe_magic = { 4D 5A } 

    condition:
        $pe_magic at 0 and (all of ($api*)) and (all of ($clip*))
}
