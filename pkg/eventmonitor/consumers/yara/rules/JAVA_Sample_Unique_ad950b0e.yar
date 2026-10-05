rule JAVA_Sample_Unique_ad950b0e
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "ad950b0e3ae00d55b83f7c0d33067b73af17f4f4bdc7bcefbe5b1dc7a9a5bc57"
        yarahub_uuid = "6d9ed471-c68d-45dc-8ad3-f4b1373ccf14"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "11d14fa43c4414efb1069163df35bcab"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/TabDetector$Mode" ascii
        $s1 = "com/nnpg/glazed/modules/main/TabDetector" ascii
        $s2 = "0[Lcom/nnpg/glazed/modules/main/TabDetector$Mode;" ascii
        $s3 = "2()[Lcom/nnpg/glazed/modules/main/TabDetector$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}
