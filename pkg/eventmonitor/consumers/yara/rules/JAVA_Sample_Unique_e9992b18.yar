rule JAVA_Sample_Unique_e9992b18
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "e9992b18ab44cbcdbab88564a495652b6da334657b3438fa127d080d54bb57cd"
        yarahub_uuid = "96ec719a-cd2b-4dda-8e4f-745ae64c1822"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b52e4c7e7db180a3345009ef46490336"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/OrderDropper$Stage" ascii
        $s1 = "com/nnpg/glazed/modules/main/OrderDropper" ascii
        $s2 = "2[Lcom/nnpg/glazed/modules/main/OrderDropper$Stage;" ascii
        $s3 = "4()[Lcom/nnpg/glazed/modules/main/OrderDropper$Stage;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}
