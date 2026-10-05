rule JAVA_Sample_Unique_4100539b
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "4100539b1b2cb6d9b9c4ad0d6cb70fefb5d4bb8c6ce1fce57219ed418d0a72cb"
        yarahub_uuid = "a2e24f57-3369-4114-a4d6-9bc2a6f66ebc"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "01aeeaba164a3c007adfc4907750d83f"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "2meteordevelopment/meteorclient/utils/entity/Target" ascii
        $s1 = "com/nnpg/glazed/modules/pvp/AimAssist$1" ascii
        $s2 = "com/nnpg/glazed/modules/pvp/AimAssist" ascii
        $s3 = "7()[Lmeteordevelopment/meteorclient/utils/entity/Target;" ascii
        $s4 = "4Lmeteordevelopment/meteorclient/utils/entity/Target;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
