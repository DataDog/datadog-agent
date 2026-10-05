rule JAVA_Sample_Unique_c1dcf57c
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "c1dcf57c3fa6db0051e260a16787efad913981ad79bf94b879501957d6d3176b"
        yarahub_uuid = "a6e2d2c3-9627-46fa-9672-c1be0aebda0a"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "4568b3fb75f5023005d9494130db7b5b"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/pvp/ShieldBreaker$ShieldBreakerState" ascii
        $s1 = "com/nnpg/glazed/modules/pvp/ShieldBreaker" ascii
        $s2 = "?[Lcom/nnpg/glazed/modules/pvp/ShieldBreaker$ShieldBreakerState;" ascii
        $s3 = "A()[Lcom/nnpg/glazed/modules/pvp/ShieldBreaker$ShieldBreakerState;" ascii
        $s4 = "ShieldBreakerState" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
