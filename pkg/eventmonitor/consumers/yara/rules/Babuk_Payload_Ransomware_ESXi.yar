rule Babuk_Payload_Ransomware_ESXi
{
    meta:
        id = "BsSWXZd0LuupvmhUQc8Tla"
        fingerprint = "91664e63f9a66b83da3ac2cf4d24a5c98f837c579415a0baa68c556ca88b4d67"
        version = "1.0"
        date = "2026-03-15"
        modified = "2026-03-15"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "Babuk Payload ransomware Linux/ESXi variant - Curve25519 + ChaCha20 encryption, FBI RC4 footer key, ESXi VM inventory targeting"
        category = "MALWARE"
        malware = "BABUK"
        malware_type = "RANSOMWARE"
        mitre_att = "T1486"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "Babuk Payload ransomware ESXi locker detected. Curve25519 ECDH + ChaCha20 file encryption targeting VMware ESXi."
        yarahub_uuid = "240635a3-ad8a-45ec-9176-815e4fa86fb2"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e0fd8ff6d39e4c11bdaf860c35fd8dc0"

    strings:
        // "FBI" fused with thread pool format string -- builder artifact
        // FBI is the 3-byte RC4 key for footer encryption, concatenated with thpool format
        $fbi_thpool = "FBIthread-pool-%d" ascii

        // ChaCha20 constant
        $chacha = "expand 32-byte k" ascii

        // Branding
        $payload = "payload" ascii

        // Ransom note extension (RECOVERY-xx0001.txt)
        $note_ext = ".xx0001" ascii

        // Anti-debug: reads TracerPid from /proc/self/status
        $antidebug = "TracerPid:" ascii

        // Self-inspection
        $proc_self = "/proc/self/exe" ascii

        // Thread pool error messages (C-Thread-Pool library)
        $thpool_err1 = "thpool_init(): Could not allocate memory for thread pool" ascii
        $thpool_err2 = "thpool_add_work(): Could not allocate memory for new job" ascii
        $thpool_err3 = "thread_do(): cannot handle SIGUSR1" ascii

    condition:
        uint32(0) == 0x464C457F
        and filesize > 20KB and filesize < 500KB
        and (
            // High confidence: FBI+thpool artifact + ChaCha20 (unique to Payload builder)
            ($fbi_thpool and $chacha)
            or
            // High confidence: FBI+thpool + payload branding + note extension
            ($fbi_thpool and $payload and $note_ext)
            or
            // Medium confidence: ChaCha20 + payload branding + anti-debug + thpool errors
            ($chacha and $payload and $antidebug and 1 of ($thpool_err1, $thpool_err2, $thpool_err3))
            or
            // Medium confidence: note ext + ChaCha20 + proc inspection + thpool
            ($note_ext and $chacha and $proc_self and 1 of ($thpool_err1, $thpool_err2))
        )
}
