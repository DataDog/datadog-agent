/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Openfind_Backdoor_ModRemote_Apache
{
    meta:
        description = "mod_remote.so: Apache HTTP backdoor, AES-128-CBC plus popen, triggered by the OpenfindMaster header"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "16a44796-b6a1-414b-81e2-d83adc7263c0"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e4045abc54e155f6bb430bbb77c9ff52"
        sha256 = "09b27d8cf5031db8554c26d60b485d9db7eaf40f2217d728f38664e87d4873f2"
        note = "The embedded MD5 is the actor's own infection marker; mgbnr.pl reads it to decide whether the host is already implanted."

    strings:
        
        $hdr = "OpenfindMaster"               ascii

        
        $f1 = "BASE64_AES_DECODE_CMDCODE"     ascii
        $f2 = "RET_ENCODE_AES_BASE64"         ascii
        $f3 = "key_find_and_run"              ascii
        $f4 = "base64_run"                    ascii

        
        $key1 = { 48 B8 4F 70 65 6E 66 69 6E 64 }
        $key2 = { 48 BA 31 32 33 21 40 23 30 30 }

    condition:
        uint32(0) == 0x464C457F               
        and (
            $hdr
            or all of ($key*)
            or 3 of ($f*)
        )
}

