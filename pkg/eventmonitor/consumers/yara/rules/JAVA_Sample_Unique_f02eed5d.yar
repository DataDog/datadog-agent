rule JAVA_Sample_Unique_f02eed5d
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "f02eed5ddcbe9b1ae6c5455ad593b790fe3bf35a8425a59dd71938c3be99efc0"
        yarahub_uuid = "d4e6db87-e3bb-409a-a5e8-8bc84f94a1dc"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "656d07e4a153b3df5fa1b8c103e0b7c8"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/PlayerDetection" ascii
        $s1 = "4[Lcom/nnpg/glazed/modules/main/PlayerDetection$Mode;" ascii
        $s2 = "6()[Lcom/nnpg/glazed/modules/main/PlayerDetection$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
