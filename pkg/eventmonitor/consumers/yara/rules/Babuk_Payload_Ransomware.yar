rule Babuk_Payload_Ransomware
{
    meta:
        id = "Fuf8T_FxfCEqvOh2npnxDo"
        fingerprint = "b95f09b1cc4c19f58e94548b2a03b6bc3321a66251b23c97ea4c934903f2c2b0"
        version = "1.0"
        date = "2026-03-15"
        modified = "2026-03-15"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "Babuk Payload ransomware - Curve25519 + ChaCha20 encryption, RC4 FBI footer key, MakeAmericaGreatAgain mutex"
        category = "MALWARE"
        malware = "BABUK"
        malware_type = "RANSOMWARE"
        mitre_att = "T1486"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "Babuk Payload ransomware detected. Curve25519 ECDH + ChaCha20 file encryption, shadow copy deletion, service/process termination."
        yarahub_uuid = "da565d14-6097-4976-bf64-c59863bfbc7d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e0fd8ff6d39e4c11bdaf860c35fd8dc0"

    strings:
        // Mutex
        $mutex = "MakeAmericaGreatAgain" wide

        // Ransom note and extension
        $note = "RECOVER_payload.txt" wide
        $ext = ".payload" wide

        // RC4 footer key adjacent to ChaCha20 constant in .rdata
        // "expand 32-byte k" + "FBI\0" (contiguous in .rdata)
        $chacha_fbi = { 65 78 70 61 6E 64 20 33 32 2D 62 79 74 65 20 6B 46 42 49 00 }

        // ASCII art banner (unique to Payload branding)
        $banner = "##m#\"  \"mm\"#" wide

        // CLI switches and config strings (wide)
        $arg_bypass = "bypass-etw" wide
        $arg_bg = "--background" wide

        // Log prefixes (wide)
        $log_enc = "[Encryption] " wide
        $log_cpu = "[CPU] Cores: " wide
        $log_mutex = "[Mutex] locker running.." wide
        $log_mode = "[Mode] Lock all drives" wide
        $log_api = "[API] Failed to init nt!" wide

        // Shadow copy deletion command
        $vss = "/c vssadmin.exe delete shadows /all /quiet" wide

        // Targeted services (ASCII, embedded in .rdata)
        $svc_veeam = "VeeamTransportSvc" ascii
        $svc_qb = "Intuit.QuickBooks.FCS" ascii
        $svc_acronis = "AcronisAgent" ascii
        $svc_360 = "zhudongfangyu" ascii

        // Pubkey size validation
        $pubkey_marker = "SIZE OF PUBKEY IS LOWER THAN NEED (32)" wide

    condition:
        uint16(0) == 0x5A4D
        and filesize > 200KB and filesize < 1MB
        and (
            // High confidence: chacha+FBI key combo (unique to this variant)
            $chacha_fbi
            or
            // High confidence: mutex + ransom note
            ($mutex and $note)
            or
            // High confidence: mutex + extension + any log prefix
            ($mutex and $ext and 1 of ($log_enc, $log_cpu, $log_mutex, $log_mode))
            or
            // Medium confidence: banner + note + vss deletion
            ($banner and $note and $vss)
            or
            // Medium confidence: 3+ log prefixes + extension + shadow deletion
            (3 of ($log_enc, $log_cpu, $log_mutex, $log_mode, $log_api) and $ext and $vss)
            or
            // Medium confidence: CLI switches + pubkey check + services
            ($arg_bypass and $pubkey_marker and 2 of ($svc_veeam, $svc_qb, $svc_acronis, $svc_360))
            or
            // Medium confidence: background flag + extension + services + vss
            ($arg_bg and $ext and 2 of ($svc_veeam, $svc_qb, $svc_acronis, $svc_360) and $vss)
        )
}
