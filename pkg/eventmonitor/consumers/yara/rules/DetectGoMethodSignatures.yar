rule DetectGoMethodSignatures {
    meta:
        description = "Detects Go method signatures in unpacked Go binaries"
        author = "Wyatt Tauber"
        date = "2024-12-03"
        yarahub_reference_md5 = "c8820b30c0eddecf1f704cb853456f37"
        yarahub_uuid = "2a5e4bcf-3fcb-4bc9-9767-352e8d3307d6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $go_signature = /[a-zA-Z_][a-zA-Z0-9_]*\.\(\*[a-zA-Z_][a-zA-Z0-9_]*\)\.[a-zA-Z_][a-zA-Z0-9_]*/

    condition:
        $go_signature
}
