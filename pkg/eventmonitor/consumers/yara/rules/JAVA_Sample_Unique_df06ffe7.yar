rule JAVA_Sample_Unique_df06ffe7
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "df06ffe7c8764bfb2e4904db3df73689c9a80c9606224fa4805147753a673f58"
        yarahub_uuid = "6afc0eb1-dd87-47ca-aa59-be7c78f658ca"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "448be01617bc3801502424822b18c0e4"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/TpaMacro$TpaType" ascii
        $s1 = "com/nnpg/glazed/modules/main/TpaMacro" ascii
        $s2 = "0[Lcom/nnpg/glazed/modules/main/TpaMacro$TpaType;" ascii
        $s3 = "2()[Lcom/nnpg/glazed/modules/main/TpaMacro$TpaType;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}
