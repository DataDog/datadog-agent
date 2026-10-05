rule JAVA_Sample_Unique_d5bb1b3f
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "d5bb1b3f6c6e2b4d2d0706d7d60f5eba8d8eb04e16b15a682f3b5d9139a71071"
        yarahub_uuid = "cad48c6f-5601-4e2b-8010-fbcc7ce6ba42"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "76bc333c0d8f51e8a6616981126d2e0e"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/foure/client/module/modules/donut/SusChunkFinder$GeodeInfo" ascii
        $s1 = "GeodeInfo" ascii
        $s2 = "blockCount" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
