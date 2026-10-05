rule JAVA_Sample_Unique_7738156e
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "7738156e8575f384d749be8ec93a468915d68c64ad8b9c784fe7640a75dd5281"
        yarahub_uuid = "69791f4b-2e23-420c-8961-3a68f480485c"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "2232d7605f7db3fc845ad6c257137dfc"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/EmergencyOrder" ascii
        $s1 = "3[Lcom/nnpg/glazed/modules/main/EmergencyOrder$Step;" ascii
        $s2 = "5()[Lcom/nnpg/glazed/modules/main/EmergencyOrder$Step;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
