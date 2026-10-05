// Last edit: 2024-11-09
rule ELF_Mirai {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-08-09"
        description = "Detects multiple Mirai variants"
        yarahub_uuid = "386c1b6c-c5f9-4a9c-a83f-1f940f4d2c2e"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "7f5d67a7309d7ff399247f1c43a92ad4"
        malpedia_family = "elf.mirai"

    strings:
        $mirai = "ATTACKRUNNING" ascii
        $mirai2 = "get_ips_in_that_block" ascii
        $mirai3 = "*/15 * * * * %s" ascii
        $mirai4 = "attack_method_" ascii
        $mirai5 = "User-Agent: Uirusu/2.0" ascii
        $mirai6 = "attack_parser" ascii
        $mirai7 = "udpplain_flood" ascii
        $mirai8 = "[0m Device Joined As [" ascii
        $mirai9 = "killall -9 telnetdbot; killall -9 Challenge; killall -9 oxdedfgt; killall -9 eBot; killall -9 lzrd" ascii
        $mirai10 = "%255s %255s %*s %255[^" ascii
        $mirai11 = "POST /ctrlt/DeviceUpgrade_1 HTTP/1.1" ascii
        $mirai12 = "echo \"Found process the and kill pid: $PID\"" ascii
        $mirai13 = "PID=$(echo $line | grep -o \"/proc/[0-9]*\" | grep -o \"[0-9]*\")" ascii
        $mirai14 = "KillerStartListener" ascii
        $mirai15 = "port_killer.c" ascii
        $mirai16 = "AttackTcpRaw" ascii
        $mirai17 = "AttackUdpRaw" ascii
        $mirai18 = ".tcp %s %s %d %s %d %d %s %d" ascii
        $mirai19 = ".udp %s %s %s %d %d %d %d" ascii
        $mirai20 = ".sudp %s %s %s %d %d %d" ascii
        $mirai21 = ".handshake %s %s %d %d" ascii
        $mirai22 = ".psh %s %s %s %d %d %d" ascii
        $mirai23 = ".syn %s %s %d %d" ascii
        $mirai24 = "setsockopt() SO_SNDTIMEO" ascii
        $mirai25 = "hostname Kamru" ascii
        $mirai26 = "(Killer) >> KILLING PID: (%s)" ascii
	$mirai27 = "telnetd|udhcpc|ntpclient|boa|httpd|mini_http|watchdog|pppd" ascii
	$mirai28 = "Found And Killed Process: PID=%d, Realpath=%s" ascii
	$mirai29 = "Killed Process: %s, PID: %d" ascii
        
        $arch = ".arm" ascii
        $arch2 = ".mpsl" ascii
        $arch3 = ".mips" ascii
        $arch4 = ".ppc" ascii
        $arch5 = ".x86" ascii
        $arch6 = ".m68k" ascii
        $arch7 = ".m88k" ascii
        $arch8 = ".sh4" ascii
        $arch9 = ".spc" ascii
        
        $archx = "_ARM" ascii
        $archx2 = "_MPSL" ascii
        $archx3 = "_MIPS" ascii
        $archx4 = "_PPC" ascii
        $archx5 = "_X86" ascii
        $archx6 = "_M68K" ascii
        $archx7 = "_M88K" ascii
        $archx8 = "_SH4" ascii
        $archx9 = "_SPC" ascii
        
        $shell_script = "\\x23\\x21\\x2F\\x62\\x69\\x6E\\x2F\\x73\\x68\\x0A\\x0A\\x66\\x6F\\x72\\x20\\x70\\x69\\x64\\x20\\x69\\x6E\\x20\\x2F\\x70\\x72\\x6F\\x63\\x2F\\x5B\\x30\\x2D\\x39" ascii
        $shell_script2 = "\\x5D\\x2A\\x3B\\x20\\x64\\x6F\\x0A\\x20\\x20\\x20\\x20\\x63\\x61\\x73\\x65\\x20\\x24\\x28\\x6C\\x73\\x20\\x2D\\x6C\\x20\\x24\\x70\\x69\\x64\\x2F\\x65\\x78\\x65" ascii
        $shell_script3 = "\\x29\\x20\\x69\\x6E\\x0A\\x20\\x20\\x20\\x20\\x20\\x20\\x20\\x20\\x2A\\x22\\x28\\x64\\x65\\x6C\\x65\\x74\\x65\\x64\\x29\\x22\\x2A\\x7C\\x2A\\x22\\x2F\\x2E\\x22" ascii
        $shell_script4 = "\\x2A\\x29\\x20\\x6B\\x69\\x6C\\x6C\\x20\\x2D\\x39\\x20\\x24\\x7B\\x70\\x69\\x64\\x23\\x23\\x2A\\x2F\\x7D\\x20\\x3B\\x3B\\x0A\\x20\\x20\\x20\\x20\\x65\\x73\\x61" ascii
        $shell_script5 = "\\x63\\x0A\\x64\\x6F\\x6E\\x65\\x0A" ascii

        // Decoded shell script:
        // #!/bin/sh
        //
        // for pid in /proc/[0-9]*; do
        //     case $(ls -l $pid/exe) in
        //         *"(deleted)"*|*"./"*)
        //             kill -9 ${pid##*/} ;;
        //     esac
        // done

    condition:
	(any of ($mirai*) or
    	4 of ($arch*) or
    	4 of ($archx*) or
    	all of ($shell_script*)) and
	uint16(0) == 0x457f
}