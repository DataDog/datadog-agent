rule JAVA_Sample_Unique_1d851c5c
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "1d851c5c5680ae6a91b317d8030a89e6b75eed5a6397a522e6c52c333ebcadcc"
        yarahub_uuid = "63f574bb-706e-49fa-96d4-e50e8bf3214d"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ae69afb07dd7b084b9f7270d8b7cad60"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "8[Lcom/nnpg/glazed/modules/main/RTPNetherBaseFinder$Mode;" ascii
        $s1 = ")[Lcom/nnpg/glazed/modules/main/RTPNetherBaseFinder$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 2 of them
}
