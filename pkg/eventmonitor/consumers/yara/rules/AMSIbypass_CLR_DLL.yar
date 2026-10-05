rule AMSIbypass_CLR_DLL{
    meta:
        id = "bf2ed8ea-db94-4025-a5d2-f65674acb8d9"
        yarahub_uuid = "c9c67fce-ff79-4e4b-a74d-b05b4b8ec78c"
        yarahub_license = "CC0 1.0"
        version = "1.0"
        malware = "Generic AMSI bypass"
        description = "AMSI bypass CLR. https://practicalsecurityanalytics.com/new-amsi-bypss-technique-modifying-clr-dll-in-memory/"
        yarahub_reference_link = "https://practicalsecurityanalytics.com/new-amsi-bypss-technique-modifying-clr-dll-in-memory/"
        source = "Sekoia.io"
        creation_date = "2025-02-28"
        date = "2025-02-28"
        classification = "TLP:WHITE"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        hash = "cd6f4fc883d86f2411809b3116629a9ef0a9f624acc31c7786db9f71dc07e5a0"
        yarahub_reference_md5 = "b30355dea8f4bcb58ac0fec0e4e1b72d"
    strings:
        $ = "EndsWith(\"clr.dll\"" ascii 
        $ = "$PAGE_READONLY = 0x02" ascii
        $ = "$PAGE_READWRITE = 0x04" ascii
        $ = "$PAGE_EXECUTE_READWRITE = 0x40" ascii
        $ = "$PAGE_EXECUTE_READ = 0x20" ascii
        $ = "$PAGE_GUARD = 0x100" ascii
        $ = "$MEM_COMMIT = 0x1000" ascii
        $ = "$MAX_PATH = 260" ascii
    condition:
        all of them
}
