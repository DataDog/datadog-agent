rule Hunt_KiraAsciiObfuscator {
    meta:
        description = "Detects files and scripts obfuscated using KiraAsciiObfuscator patterns and structural artifacts"
        author = "Serhii Kocherhan"
        date = "2026-09-04"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "e6390a0c-4332-4a62-82a6-61572362b42e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "12adad75e1617e5274f4a93f293bfa75"

    strings:
        // Tool Signature & Header Strings
        $tool_tag1 = "KiraAsciiObfuscator" ascii wide nocase
        $tool_tag2 = "github.com/Kira" ascii wide nocase

        // Batch / CMD Obfuscation Wrappers
        $cmd_chcp  = "@echo off" ascii wide nocase
        $cmd_hex1  = "\\x" ascii wide
        $cmd_hex2  = "%x%" ascii wide

        // Distinctive ASCII/Hex Chr() and String Concatenation Chains
        // e.g., [char]0x... or Chr(0x...) repetitive loops
        $ascii_chain1 = "[char]0x" ascii wide nocase
        $ascii_chain2 = "\\x70\\x6f\\x77\\x65\\x72\\x73\\x68\\x65\\x6c\\x6c" ascii wide nocase // "\x70\x6f\x77\x65\x72\x73\x68\x65\x6c\x6c" = "powershell"
        $ascii_chain3 = "\\x63\\x6d\\x64\\x2e\\x65\\x78\\x65" ascii wide nocase              // "\x63\x6d\x64\x2e\x65\x78\x65" = "cmd.exe"

        // Python Unescape / Decoder Stub Signatures
        $py_decode1 = "bytes.fromhex(" ascii wide
        $py_decode2 = ".decode('utf-8')" ascii wide
        $py_decode3 = "codecs.decode(" ascii wide

    condition:
        filesize < 5MB and
        (
            // Primary Match: Explicit tool string or repository attribution
            any of ($tool_tag*) or

            // Secondary Match: High concentration of hex/ASCII escape sequences for shell commands
            (
                any of ($ascii_chain2, $ascii_chain3)
            ) or

            // Fallback Match: Combination of batch execution wrappers and dense hex escape chains
            (
                $cmd_chcp and 
                any of ($cmd_hex1, $cmd_hex2) and 
                any of ($ascii_chain1, $py_decode1, $py_decode2, $py_decode3)
            )
        )
}