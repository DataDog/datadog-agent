rule ELF_DDoS_Gafgyt_Variant {
    meta:
        description = "Detects Gafgyt (Qbot) / Mirai variant with specific DDoS functions (vseattack, ftcp, SendHTTPHex) and recon routines"
        author = "Serhii Kocherhan"
        date = "2026-08-17"
        yarahub_uuid = "0038778a-efd0-4bd6-a156-ed8f0de1a9c2"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "33ff73c7bc6890c4df1f0668a21a2183"

    strings:
        // ELF Header Magic Bytes (\x7fELF)
        $elf_magic = { 7F 45 4C 46 }

        // C2 Handler and DDoS Vector Function Symbols/Strings
        $func_cmd   = "processCmd" ascii fullword
        $func_udp   = "SendUDP" ascii fullword
        $func_tcp   = "ftcp" ascii fullword
        $func_http1 = "SendHTTPHex" ascii fullword
        $func_http2 = "sendHTTPtwo" ascii fullword
        $func_vse   = "vseattack" ascii fullword

        // Reconnaissance and Network Inspection
        $recon_route = "/proc/net/route" ascii
        $recon_py    = "python" ascii fullword
        $recon_perl  = "perl" ascii fullword
        $recon_telnet= "telnetd" ascii fullword

    condition:
        // Must be an ELF binary
        $elf_magic at 0 and filesize < 5MB and
        (
            // Matches if the command processor and key DDoS attack vectors are present
            ($func_cmd and 3 of ($func_udp, $func_tcp, $func_http1, $func_http2, $func_vse))
            or
            // Or matches the combination of DDoS vectors and system recon behavior
            (3 of ($func_*) and $recon_route and 2 of ($recon_py, $recon_perl, $recon_telnet))
        )
}