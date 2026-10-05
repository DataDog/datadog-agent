rule JAVA_Sample_Unique_65d3a296
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "65d3a2963c351b4a3f388e22131c22c2816e5230bec99a3ce665f4db8ad70459"
        yarahub_uuid = "66481bec-5882-4d20-9ba2-835a3e8bd94d"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e201c03b9d335faa9102198491f9f62e"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "client/xenon/imixin/IKeybinding" ascii
        $s1 = "Xenon$isActuallyPressed" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 2 of them
}
