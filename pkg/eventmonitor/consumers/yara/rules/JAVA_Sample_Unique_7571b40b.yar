rule JAVA_Sample_Unique_7571b40b
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "7571b40b2a74a205205c0b03e9e4c114741ff1b684311e01c4ffa3d213f8ba54"
        yarahub_uuid = "05494cc2-8954-444d-9715-47975b046da0"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "bd6365db84b64d9709358f642dffb9ed"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/utils/GlazedWebhook$1" ascii
        $s1 = "com/nnpg/glazed/utils/GlazedWebhook" ascii
        $s2 = "Glazed-Webhook-" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
