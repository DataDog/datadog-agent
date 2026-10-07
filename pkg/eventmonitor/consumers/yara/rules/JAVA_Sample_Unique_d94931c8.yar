rule JAVA_Sample_Unique_d94931c8
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "d94931c819880c4932dbb2a1cf2c6713fb08a10c8b65419462a58d19f821b09e"
        yarahub_uuid = "433f520a-372a-42ec-8790-558053991b4f"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "68f8a2e36a14e5cd15352fedff2ab905"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "RuntimeVisibleAnnotations&" ascii
        $s1 = "AnnotationDefault" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 2 of them
}
