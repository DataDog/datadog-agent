rule S7Flip
{
    meta:
        author = "Jeans"
        description = "Detects S7Flip Siemens S7 PLC manipulation malware"
        family = "S7Flip"
        date = "2026-09-12"

        yarahub_reference_md5 = "cdf8f8a09a04c3c2e1c0e75e4a21d7da"
        yarahub_uuid = "e5b7e188-f918-4784-ad31-46a32d09cb03"
        yarahub_license = "CC0 1.0"

        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $s1 = "aphp.exe" ascii wide
        $s2 = "logs.csv" ascii wide
        $s3 = "TIA_Portal" ascii wide
        $s4 = "randomizeSiemensParameter" ascii
        $s5 = "findAndModifyInputs" ascii
        $s6 = "connectToSiemensPLC" ascii
        $s7 = "TS7Client" ascii
        $s8 = "DBRead" ascii
        $s9 = "DBWrite" ascii

    condition:
        5 of them
}
