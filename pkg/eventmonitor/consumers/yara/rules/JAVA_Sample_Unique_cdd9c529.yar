rule JAVA_Sample_Unique_cdd9c529
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "cdd9c529ef166dc183bff2d21ac548f874da4e7258b9944951b63ca506f0c955"
        yarahub_uuid = "734b6aca-a5f6-4725-bd8a-27a514c7f010"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "82868db9ffd4839b144703506cb1ed7e"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/AutoSell$SellMode" ascii
        $s1 = "com/nnpg/glazed/modules/main/AutoSell" ascii
        $s2 = "1[Lcom/nnpg/glazed/modules/main/AutoSell$SellMode;" ascii
        $s3 = "3()[Lcom/nnpg/glazed/modules/main/AutoSell$SellMode;" ascii
        $s4 = "SellMode" ascii
        $s5 = "Whitelist" ascii
        $s6 = "Blacklist" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
