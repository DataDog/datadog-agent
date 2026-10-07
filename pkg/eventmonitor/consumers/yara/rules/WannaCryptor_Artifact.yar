rule WannaCryptor_Artifact {
    meta:
        author = "blade391off"
        description = "Detects WannaCryptor malware"
        date = "2026-09-26"
        yarahub_uuid = "77500e40-8638-4f40-b408-b762f1b2e666"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "84C82835A5D21BBCF75A61706D8AB549"

    strings:
        
        $artifact_1 = "C:\\Users\\admin\\AppData\\Local\\Temp\\taskdl.exe" ascii wide nocase
        $artifact_2 = "C:\\Users\\admin\\AppData\\Local\\Temp\\u.wnry" ascii wide nocase
        $artifact_3 = "C:\\Users\\admin\\AppData\\Local\\Temp\\taskse.exe" ascii wide nocase
        $artifact_4 = "HKEY_CURRENT_USER\\Software\\WanaCrypt0r" ascii wide nocase
        $artifact_5 = "HKEY_CURRENT_USER\\Software\\Google\\Chrome\\ThirdParty" ascii wide nocase
        $artifact_6 = "HKEY_CURRENT_USER\\Software\\Google\\Chrome\\BLBeacon" ascii wide nocase

    condition:
        
        uint16(0) == 0x5A4D and 3 of ($artifact_*)
}
