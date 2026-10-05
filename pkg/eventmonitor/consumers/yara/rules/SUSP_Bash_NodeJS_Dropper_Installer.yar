rule SUSP_Bash_NodeJS_Dropper_Installer {
    meta:
        description = "Script bash dropper : installe Node.js portable si absent (prepare un payload JS/jsc)"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / installateurs node"
        confidence  = "medium"
        yarahub_uuid            = "ca2e7b82-4259-40dd-bfc7-f9dbfce8d718"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "4b0948d074a98346e4477884a09604e1"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Node.js not found globally" ascii wide
        $b = "Attempting to download portable version" ascii wide
    condition:
        all of them
}

