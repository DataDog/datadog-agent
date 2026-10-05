rule TinyLoad_PxTR_Guard {
    meta:
        description = "Detects TinyLoad v7.1+ packed executables via unique .pxkey guard block (C0DE1337 / B007DEAD)"
        author      = "TinyLoad"
        date        = "2026-06-16"
        reference   = "https://github.com/iamsopotatoe-coder/TinyLoad"
        yarahub_uuid = "374cd9d2-36b0-48c8-902c-0d05c86ce067"
        yarahub_reference_md5 = "bcdd61adf6aa85b60e7a1168c9b78bef"
        yarahub_reference_link = "https://github.com/iamsopotatoe-coder/TinyLoad"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $guard = { 37 13 DE C0 [16] AD DE 07 B0 }
    condition:
        $guard
}
