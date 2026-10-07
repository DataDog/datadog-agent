rule DelBat1
{
    meta:
        author = "Madhav"
        description = "This is a bat file which deletes the malicious file after the malicious files are executed"
        date = "2025-06-02"
	yarahub_reference_md5 = "0CCD4E0F8639AB3DB3C45B2768A41AFB"
	yarahub_uuid = "58ff8b5e-192e-4144-af8e-f29d282d1c70"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
        $mal1 = "chcp 65001"
        $mal2 = "del /a /q /f"
        $mal3 = "\\AppData\\Local\\Temp\\"
        $mal4 = ".exe"
        $mal5 = ".bat"
          
    condition:
        ($mal1 and $mal2 and $mal3 and $mal4 and $mal5) or ($mal2 and $mal3 and $mal4 and $mal5)
}
