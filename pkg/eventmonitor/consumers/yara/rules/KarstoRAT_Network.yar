rule KarstoRAT_Network
{
    meta:
        id = "dJKjLzlU7wSFBukw8hrcdp"
        fingerprint = "10df86fa1b2ff3ae8e0f08bb7a0e4edcbdda891ce2312fd9c5a190fec71e8122"
        version = "1.0"
        date = "2026-03-10"
        modified = "2026-03-10"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "derp.ca"
        description = "KarstoRAT C2 protocol indicators - PCAP, memory, or proxy logs"
        category = "MALWARE"
        malware = "KARSTORAT"
        malware_type = "RAT"
        mitre_att = "T1071.001"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "KarstoRAT C2 beacon or command traffic detected."
        yarahub_uuid = "aa74e89c-20d2-4a52-a915-4f398729e58e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a5bef919eb260af5bb8eba243ed4fd75"

    strings:
        // User-Agent header in HTTP traffic
        $ua_header = "User-Agent: SecurityNotifier" ascii nocase

        // C2 beacon endpoints in traffic
        $beacon = "/notify?event=heartbeat&user=" ascii
        $log = "/notify?event=log&user=" ascii

        // Data exfiltration endpoints
        $exfil_screen = "/upload-screen" ascii
        $exfil_keylog = "/upload-keylog" ascii
        $exfil_sysinfo = "/upload-sysinfo" ascii
        $exfil_shell = "/upload-shell-output" ascii
        $exfil_webcam = "/upload-webcam" ascii
        $exfil_audio = "/upload-audio" ascii
        $exfil_clipboard = "/upload-clipboard" ascii
        $exfil_tokens = "/upload-tokens" ascii

        // Command retrieval
        $cmd_upload = "/client-upload?user=" ascii
        $cmd_upload_bug = "/client-upload&filename=" ascii
        $cmd_shell_input = "/get-shell-input" ascii

        // Known C2 infrastructure
        $c2_ip = "212.227.65.132" ascii wide
        $c2_ngrok = "hallucinative-shabbily-olga.ngrok-free.dev" ascii wide

        // External IP check
        $ipcheck = "http://api.ipify.org" ascii

    condition:
        // Not PE-only - works on PCAP, memory dumps, proxy logs
        (
            // C2 beacon traffic
            ($ua_header and ($beacon or $log))
            or
            // Data exfiltration traffic
            ($ua_header and 2 of ($exfil_*))
            or
            // C2 protocol endpoints in any file
            ($beacon and $log and 3 of ($exfil_*))
            or
            // Known infrastructure IOCs with protocol
            (($c2_ip or $c2_ngrok) and ($beacon or $log or 2 of ($exfil_*)))
            or
            // Upload endpoints (normal + buggy) with exfiltration
            (($cmd_upload or $cmd_upload_bug) and $cmd_shell_input and 2 of ($exfil_*))
            or
            // IP check + beacon + exfiltration
            ($ipcheck and ($beacon or $log) and 2 of ($exfil_*))
        )
}
