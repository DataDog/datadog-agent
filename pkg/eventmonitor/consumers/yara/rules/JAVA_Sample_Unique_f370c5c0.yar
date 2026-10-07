rule JAVA_Sample_Unique_f370c5c0
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "f370c5c0dde3a04fc029cdf3889a44dcf1060feff47a283fde48deb40f09c3f5"
        yarahub_uuid = "9c4bc444-616b-4972-bd23-8df6b4080d45"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "71c92cd0b8ae72ef2d1f5d4d2f2faa19"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/esp/PistonESP" ascii
        $s1 = "3[Lcom/nnpg/glazed/modules/esp/PistonESP$PistonType;" ascii
        $s2 = "5()[Lcom/nnpg/glazed/modules/esp/PistonESP$PistonType;" ascii
        $s3 = "PistonType" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}
