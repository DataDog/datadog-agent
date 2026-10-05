rule JAVA_Sample_Unique_d2a6ca38
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "d2a6ca38feda2c859d03c595b5527e89d29f75a6d562d35e09b86bce1ceea5aa"
        yarahub_uuid = "a4cc464b-198c-455f-acf5-a171e2a16759"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "8d23d0b4f21c3e723c7bd3a2e6a829f7"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/utils/hud/AdminBoard$Line" ascii
        $s1 = "com/nnpg/glazed/utils/hud/AdminBoard" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 2 of them
}
