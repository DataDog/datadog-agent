rule KarstoRAT_Token_Stealer
{
    meta:
        id = "eNE3ujpchWIRzIwa8ch7D5"
        fingerprint = "03d56093829e480b2bf9d69f01a0b354f3de40c54708ab885e62d97d4477dc72"
        version = "1.0"
        date = "2026-03-10"
        modified = "2026-03-10"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "derp.ca"
        description = "KarstoRAT token stealer component - Discord + browser credential theft (Builds 2+)"
        category = "MALWARE"
        malware = "KARSTORAT"
        malware_type = "INFOSTEALER"
        mitre_att = "T1528"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "KarstoRAT token stealer targeting Discord and browser credentials detected."
        yarahub_uuid = "bb160c66-5a11-45f1-87e0-d086c7989976"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a5bef919eb260af5bb8eba243ed4fd75"

    strings:
        $ua = "SecurityNotifier" ascii wide

        // Discord token paths
        $discord = "\\discord\\Local Storage\\leveldb\\" ascii wide
        $discord_canary = "\\discordcanary\\Local Storage\\leveldb\\" ascii wide
        $discord_ptb = "\\discordptb\\Local Storage\\leveldb\\" ascii wide

        // Browser token paths
        $chrome = "\\Google\\Chrome\\User Data\\Default\\Local Storage\\leveldb\\" ascii wide
        $brave = "\\BraveSoftware\\Brave-Browser\\User Data\\Default\\Local Storage\\leveldb\\" ascii wide
        $edge = "\\Microsoft\\Edge\\User Data\\Default\\Local Storage\\leveldb\\" ascii wide
        $opera = "\\Opera Software\\Opera Stable\\Local Storage\\leveldb\\" ascii wide

        // Token regex pattern (embedded in binary)
        $token_regex = "[a-zA-Z0-9_-]{23,28}\\.[a-zA-Z0-9_-]{6}\\.[a-zA-Z0-9_-]{25,110}|mfa\\.[a-zA-Z0-9_-]{84}" ascii

        // Token upload endpoint
        $upload_tokens = "/upload-tokens" ascii

        // TOKEN_GRAB command
        $cmd_tokengrab = "TOKEN_GRAB" ascii wide

        // Action responses
        $resp_found = "Action: [TOKEN_GRAB] -> Found " ascii
        $resp_none = "Action: [TOKEN_GRAB] -> No tokens found" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize > 50KB and filesize < 500KB
        and (
            // Discord + browser paths + regex = token stealer
            (2 of ($discord*) and 2 of ($chrome, $brave, $edge, $opera) and $token_regex)
            or
            // Token stealer with C2 upload
            ($cmd_tokengrab and $upload_tokens and 2 of ($discord*))
            or
            // User-Agent + token regex + upload = KarstoRAT stealer
            ($ua and $token_regex and $upload_tokens)
            or
            // Token grab responses + paths
            (($resp_found or $resp_none) and 2 of ($discord*))
        )
}
