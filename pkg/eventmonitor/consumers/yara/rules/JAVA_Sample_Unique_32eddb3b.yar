rule JAVA_Sample_Unique_32eddb3b
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "32eddb3bc74a8c28451e37e671c42b501a6dd6435fb5407a38cb9c53dd1ae93b"
        yarahub_uuid = "f31f0340-8825-458e-a700-384067827153"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "1f0eb6297e80a8c49a25b167077584f6"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/esp/LamaESP$Mode" ascii
        $s1 = "com/nnpg/glazed/modules/esp/LamaESP" ascii
        $s2 = ")[Lcom/nnpg/glazed/modules/esp/LamaESP$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
