rule JAVA_Sample_Unique_41362105
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "4136210525cd009e8e12eb67c9c094d701ae81ef8961450757411c2a9b5ea2b1"
        yarahub_uuid = "76340da7-41a8-4677-a9a0-a528f491b8ca"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e95738eb613c172ec6c3dece8a31bb23"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/utils/glazed/StringUtils" ascii
        $s1 = "lambda$convertUnicodeToAscii$0" ascii
        $s2 = "convertUnicodeToAscii" ascii
        $s3 = "buildHomoglyphs" ascii
        $s4 = "codePoints" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
