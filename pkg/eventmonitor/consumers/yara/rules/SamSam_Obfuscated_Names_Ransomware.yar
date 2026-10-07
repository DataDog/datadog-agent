rule SamSam_Obfuscated_Names_Ransomware {
    meta:
        description = "Obfuscated .NET ransomware (SamSam-like): stretched function names - recursive drive encryption, process kill, hex routines"
        author      = "Marjoriefort"
        date        = "2026-09-15"
        reference   = "InTheWild.0438 miss / 1a4280aaab205b8e073be8f4c170e2ed_elex_samsam"
        confidence  = "high"
        yarahub_uuid            = "37d300a2-2e59-4186-b81f-cfaf07cfb800"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "1a4280aaab205b8e073be8f4c170e2ed"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "goforrecursfindfilesinadrive"
        $b = "runerforcheckifprocisopen"
        $c = "ru_n_p_rgo_ra_mby_a_rg_"
        $d = "contentchekerrrrrrrr"
    condition:
        2 of them
}
