rule JAVA_Sample_Unique_23a90a56
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "23a90a56c18312cdc22f1836956e30db37f10adde18ccd1ff367957116d92f32"
        yarahub_uuid = "9bfa9aea-63c7-4c34-93c9-d956f4baa311"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "bf9b7ddb407701b21ff87f686c564a41"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/SpawnerOrder$State" ascii
        $s1 = "com/nnpg/glazed/modules/main/SpawnerOrder" ascii
        $s2 = "2[Lcom/nnpg/glazed/modules/main/SpawnerOrder$State;" ascii
        $s3 = "4()[Lcom/nnpg/glazed/modules/main/SpawnerOrder$State;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}
