rule MAL_Linux_IoT_MultiArch_BotnetLoader_Generic
{
    meta:
        author = "Anish Bogati"
        description = "Technique-based detection of IoT/Linux botnet loader shell scripts downloading binaries from numeric IPs, chmodding, and executing multi-architecture payloads"
        date = "2025-12-01"
        yarahub_reference_md5       = "e72fbbe6906052e1d8f3546644602849"
        yarahub_uuid                = "4b0e3b57-6d91-4c3a-8f5d-0d7c6b2ff101"
        yarahub_license             = "CC0 1.0"
        yarahub_rule_matching_tlp   = "TLP:WHITE"
        yarahub_rule_sharing_tlp    = "TLP:WHITE"
        reference                   = "MalwareBazaar sample lilin.sh"
        yarahub_reference_link      = "https://bazaar.abuse.ch/sample/8cc4dbecbbd2d5dcd4722a63b936a694893aefa99db815284117a325d19f2fdc/"
        reference_sha256            = "8cc4dbecbbd2d5dcd4722a63b936a694893aefa99db815284117a325d19f2fdc"


    strings:
        // --- Technique: Downloaders ---
        $wget1    = "wget http://" ascii
        $wget2    = "wget -q http://" ascii
        $bb_wget  = "busybox wget" ascii
        $curl1    = "curl -fsS" ascii
        $curl2    = "curl -fsSO http://" ascii

        // Numeric IPv4 URL with optional port and path
        $re_ip_http = /http:\/\/[0-9]{1,3}(\.[0-9]{1,3}){3}(:[0-9]{1,5})?\/[A-Za-z0-9._\-\/]+/ ascii

        // --- Technique: Make downloaded files executable ---
        $chmod1  = "chmod 777" ascii
        $chmod2  = "chmod +x" ascii

        // --- Multi-arch evidence in FILENAMES / EXTENSIONS ---
        // e.g. Fantazy.mips, rondo.armv7l, irannet.mipsel, *.x86_64, *.sparc, *.arc700, *.i686, *.spc
        $re_ext_arch = /\.(mips(el)?|arm(v[4567]l|5|6|7)?|x86(_64)?|m68k|ppc|sparc|spc|sh4|arc700|i[3456]86)\b/ ascii

        // --- Multi-arch evidence in EXECUTION lines ---
        // e.g. ./mips, ./x86_64, ./Fantazy.mips, ./irannet.mipsel, ./arm7, ./ppc
        $re_exec_arch = /\.\/[A-Za-z0-9._\-]*(mips(el)?|arm(v[4567]l|5|6|7)?|x86(_64)?|m68k|ppc|sparc|spc|sh4|arc700|i[3456]86)\b/ ascii

    condition:
        // Script-like file, not a binary
        filesize < 50KB and
        uint32(0) != 0x7F454C46 and  // ELF
        uint16(0) != 0x4D5A and      // MZ (PE)

        // 1) Downloader present
        ( $wget1 or $wget2 or $bb_wget or $curl1 or $curl2 ) and

        // 2) Numeric-IP HTTP URLs present
        #re_ip_http >= 1 and

        // 3) chmod to make payload executable
        ( $chmod1 or $chmod2 ) and

        // 4) Strong multi-arch evidence:
        //    sum of filename-arch matches + exec-arch matches >= 2
        ( #re_ext_arch + #re_exec_arch ) >= 2
}