rule JAVA_Sample_Unique_2d57e5bd
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "2d57e5bd257beff2046b80b8e25c0138489c212dbb1215cbf43406badda1dc8f"
        yarahub_uuid = "6d0333d3-d160-487d-bd99-12b507302597"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "605f1ef7163498d24b06bfb95577a148"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/esp/DrownedTridentESP" ascii
        $s1 = ")[Lcom/nnpg/glazed/modules/esp/DrownedTridentESP$RenderMode;" ascii
        $s2 = "RenderMode" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
