rule JAVA_Sample_Unique_130cb3c6
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "130cb3c62512c2817937a7e05a9093b4d46d3fb18bf4d81df2bb28a9017ba3fc"
        yarahub_uuid = "47ef5a69-da84-4f12-83a0-4bc41d0147b1"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d478e2e6a5d82c83e8cf2761a9a2731f"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/foure/client/module/modules/render/LightESP$LightBlock" ascii
        $s1 = "com/foure/client/module/modules/render/LightESP" ascii
        $s2 = "Lnet/minecraft/class_2338;I)V" ascii
        $s3 = "LightBlock" ascii
        $s4 = "lightLevel" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
