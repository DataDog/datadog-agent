rule Windows_Ransomware_OK75_r543t_AutoIt
{
    meta:
        author = "ShadowOpCode"
        description = "Detects the r543t/OK75 AutoIt ransomware lineage; includes a packed fingerprint for relfixed3.exe and family indicators for decoded/tokenized AutoIt script content"
        date = "2026-09-28"
        family = "r543t/OK75"
        sample_sha256 = "5af82a82ca5a827e525f0bb5c7d2639accca65c608c0cb1b0a7f561c597c78f0"
        yarahub_reference_link = "https://github.com/ShadowOpCode/r543t-ok75-ransomware-recovery"
        yarahub_reference_md5 = "3bdf349bce3359feb0c1ab93a17ce378"
        yarahub_uuid = "ac35edf4-d1ba-485b-91b6-34adc66a714c"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        /*
         * Raw compiled build fingerprint.
         * The AutoIt script is compressed and LAME-encrypted inside the SCRIPT
         * RCDATA resource, so these anchors intentionally identify the analyzed
         * relfixed3 build rather than pretending to be stable family features.
         */
        $autoit_ea06 = { A3 48 4B BE 98 6C 4A A9 99 4C 53 0A 86 D6 48 7D 41 55 33 21 45 41 30 36 }
        $packed_1 = { 11 B2 0F 1C 13 6D 73 41 FF 0E 1A F7 18 74 74 9B D7 5F D9 A8 ED 60 0E 8F }
        $packed_2 = { 0E C7 FF 03 BC 96 55 8D BF 49 9F 78 B5 95 92 ED E7 A2 F5 DA A4 3C 0B D9 }
        $packed_3 = { 5D 61 C9 64 71 A1 42 7D 22 84 16 61 F5 91 6D F5 5F 28 0A D8 46 23 97 0A }
        $packed_4 = { 74 B8 54 22 C0 8D 06 E2 F2 32 12 59 EB CC 26 88 AC EE 30 11 36 12 5C F7 }

        /* Decompiled/decoded AutoIt source indicators. */
        $src_cryptppwd     = "CRYPTPPWD" ascii wide nocase
        $src_dencrypted2   = "DENCRYPTED2" ascii wide nocase
        $src_target_list   = "y.txt" ascii wide nocase
        $src_key_handle    = "sslog.txt" ascii wide nocase
        $src_ext_ok75      = ".ok75" ascii wide nocase
        $src_ext_r543t     = ".r543t" ascii wide nocase
        $src_derive        = "_CRYPT_DERIVEKEY" ascii wide nocase
        $src_encrypt       = "_CRYPT_ENCRYPTDATA" ascii wide nocase
        $src_aes256        = "CALG_AES_256" ascii wide nocase

        /*
         * AutoIt EA06 token-stream representations of the same family features.
         * These survive recompilation as long as the relevant source identifiers
         * and literals remain unchanged, even though the outer SCRIPT resource
         * is compressed/encrypted.
         */
        $tok_cryptppwd   = { 33 09 00 00 00 4A 00 5B 00 50 00 59 00 5D 00 59 00 59 00 5E 00 4D 00 }
        $tok_dencrypted2 = { 33 0B 00 00 00 4F 00 4E 00 45 00 48 00 59 00 52 00 5B 00 5F 00 4E 00 4F 00 39 00 }
        $tok_target_list = { 36 05 00 00 00 7C 00 2B 00 71 00 7D 00 71 00 }
        $tok_key_handle  = { 36 09 00 00 00 7A 00 7A 00 65 00 66 00 6E 00 27 00 7D 00 71 00 7D 00 }
        $tok_ext_ok75    = { 36 05 00 00 00 2B 00 6A 00 6E 00 32 00 30 00 }
        $tok_ext_r543t   = { 36 06 00 00 00 28 00 74 00 33 00 32 00 35 00 72 00 }
        $tok_derive      = { 34 10 00 00 00 4F 00 53 00 42 00 49 00 40 00 44 00 4F 00 54 00 55 00 42 00 59 00 46 00 55 00 5B 00 55 00 49 00 }
        $tok_encrypt     = { 34 12 00 00 00 4D 00 51 00 40 00 4B 00 42 00 46 00 4D 00 57 00 5C 00 51 00 40 00 4B 00 42 00 46 00 56 00 53 00 46 00 53 00 }
        $tok_aes256      = { 33 0C 00 00 00 4F 00 4D 00 40 00 4B 00 53 00 4D 00 49 00 5F 00 53 00 3E 00 39 00 3A 00 }

    condition:
        /* Exact-ish packed build: resilient to superficial PE changes, not to script recompilation. */
        (
            uint16(0) == 0x5A4D and
            $autoit_ea06 and
            3 of ($packed_*)
        )
        or
        /* Decompiled AutoIt source: family-oriented branch. */
        (
            $src_cryptppwd and
            $src_dencrypted2 and
            $src_target_list and
            $src_key_handle and
            1 of ($src_ext_*) and
            2 of ($src_derive, $src_encrypt, $src_aes256)
        )
        or
        /* Decompressed EA06 token stream: family-oriented branch. */
        (
            $tok_cryptppwd and
            $tok_dencrypted2 and
            $tok_target_list and
            $tok_key_handle and
            1 of ($tok_ext_*) and
            2 of ($tok_derive, $tok_encrypt, $tok_aes256)
        )
}
