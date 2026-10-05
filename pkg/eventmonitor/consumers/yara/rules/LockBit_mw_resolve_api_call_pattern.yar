rule LockBit_mw_resolve_api_call_pattern
{
    meta:
        description               = "Detects LockBit sample via repeated mw_resolve_api call pattern (PUSH 4 + PUSH 0x439c7e33)"
        author                    = "Aldair Maihuiri"
        date                      = "2026-08-06"
        sha256                    = "45c317200e27e5c5692c59d06768ca2e7eeb446d6d495084f414d0f261f75315"
        malware_family            = "LockBit"
        sample_type               = "ransomware"
        arch                      = "x86 32-bit"
        yarahub_uuid              = "c4e4f132-881e-4f7f-87ba-fd20f20e0ae9"
        yarahub_author_twitter    = "@AldairMaihuiri"
        yarahub_reference_link    = "https://ginomaihuiri.github.io/lockbit-string-deobfuscation"
        yarahub_license           = "CC BY-NC-SA 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "aacef4e2151c264dc30963823bd3bb17"

    strings:
        $call_resolve = { 6a 04 68 33 7e 9c 43 }

    condition:
        uint16(0) == 0x5A4D
        and filesize < 2MB
        and #call_resolve >= 10
}
