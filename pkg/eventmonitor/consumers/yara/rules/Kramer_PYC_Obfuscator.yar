rule Kramer_PYC_Obfuscator
{
    meta:
        id = "r68QjRG1X42Zqal0mtOs85"
        fingerprint = "c107955cdfa413d14cdcb1d4bd5971352f2bc5db4bf5465a804621049446464e"
        version = "1.0"
        date = "2026-03-31"
        modified = "2026-03-31"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "Kramer Python bytecode obfuscator used by SERPENTINE#CLOUD -- encodes source via CJK Unicode offsets, eval at runtime"
        category = "MALWARE"
        malware = "KRAMER"
        mitre_att = "T1027.013"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "Kramer Python bytecode obfuscator detected."
        yarahub_uuid = "79adf976-9d41-4e2d-a2b3-0a4e340951e5"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c923a78e3d3da6be010578dbd7c126d1"

    strings:
        // Kramer class name in bytecode string table
        $class_name = "Kramer" ascii

        // Decode method qualified name
        $decode_method = "Kramer.__decode__" ascii

        // Init method qualified name
        $init_method = "Kramer.__init__" ascii

        // Lambda in init (Kramer-specific nested decode logic)
        $init_lambda = "Kramer.__init__.<locals>.<lambda>" ascii

        // Instance attribute names stored in bytecode co_names
        $attr_encode = "_encode" ascii
        $attr_bits = "_bits" ascii
        $attr_sparkle = "_sparkle" ascii

    condition:
        // Python 3.12 pyc magic (cb 0d 0d 0a little-endian)
        uint32(0) == 0x0A0D0DCB
        and filesize > 500KB and filesize < 20MB
        and $class_name
        and $decode_method
        and $init_method
        and $init_lambda
        and 2 of ($attr_encode, $attr_bits, $attr_sparkle)
}
