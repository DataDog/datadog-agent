rule JAVA_Sample_Unique_3f24ff88
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "3f24ff88e4715b8291f56bb8eadbaf304585b2d1af01073e09eed73d3db73f8b"
        yarahub_uuid = "79ceb325-53ef-4fe3-9524-f1970a5e114e"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b350e66529f52c48804a3b50793a8f95"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/SpawnerProtect" ascii
        $s1 = "4[Lcom/nnpg/glazed/modules/main/SpawnerProtect$State;" ascii
        $s2 = "6()[Lcom/nnpg/glazed/modules/main/SpawnerProtect$State;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
