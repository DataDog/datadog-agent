rule LIN_Sample_Unique_9c4c8bec {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "9c4c8bec0ec8744c3a587fcb7c796443aed72eff2029bc4ccdcf27b7ee483759.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "8a1b39db-808a-49fe-9463-6e0d66d7e0e3"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "068a6cf45d553699a53a9bbe0922ac63"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "n=\"KZVR AKPQ SQCT OBGT MOFR HDYG YZZY TFKY XYGO FXTH IFRG VIGE HZCY TPDQ FYRX WFCK\"" ascii wide
        $b = "    chmod +x $a" ascii wide
    condition:
        all of them
}
