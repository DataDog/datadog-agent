rule JAVA_Sample_Unique_2cb19053
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "2cb1905399e60104940c709351a018e292554dfecdbaea4bf5f3e44ddf27c857"
        yarahub_uuid = "803ea34f-e836-4777-a44a-18217ff2d356"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f45df0fb3afefdb44b6eab3c7aeb6b19"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/foure/client/module/modules/donut/BedrockHoleESP$HoleInfo" ascii
        $s1 = "HoleInfo" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 2 of them
}
