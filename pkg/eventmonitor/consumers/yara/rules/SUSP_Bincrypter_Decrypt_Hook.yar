rule SUSP_Bincrypter_Decrypt_Hook
{
    meta:
        description = "bincrypter decryption hook (decoded stage 2, or unobfuscated build)"
        author = "Peter"
        date = "2026-09-18"
        reference = "https://github.com/hackerschoice/bincrypter"
        yarahub_uuid = "141cbc41-7e41-4569-806e-6f751acd82ce"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9f81e196548efc844bd8c407a0be834c"
        reference_filename = "stage2.sh (decoded stage 2)"

    strings:
        $esc  = { 73 2F 42 33 2F 5C 6E 2F 67 }  // s/B3/\n/g
        $esc2 = "s/B1/"
        $esc3 = "s/B2/B/g"
        $enc  = "s/B/B2/g"
        $aes  = "openssl enc -d -aes-256-cbc -md sha256 -nosalt -k"
        $fn   = "_bc_dec"
        $err  = "BC_FN=FileName source FileName"

    condition:
        filesize < 10485760
        and ( ( $esc and $esc2 and $esc3 ) or $enc )
        and 1 of ($aes, $fn, $err)
}
