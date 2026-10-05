/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Openfind_Backdoor_DeployScript
{
    meta:
        description = "mgbnr.pl: root-level deployment script (SUID binary, sudoers entry, kernel module, Apache module)"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "b56720df-765c-4a94-9337-98596abba955"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ea0b74a66e55cc01bc46beec4c7e5424"
        sha256 = "660b0842455b256579e49d1e9f62f29ca9d79aed1983adf4f5b9803e5dfb26b7"
        note = "Deliberately keyed on install verbs plus removal anchors so that defender IR runbooks quoting the IOCs do not match. Cost: a decoy '# rmmod libusb' comment inside a deploy script suppresses this rule. Script-layer detection is fragile by nature; host state is the durable layer."

    strings:
        
        $sudo   = "webmail ALL=(ALL) NOPASSWD: ALL"          ascii
        $suid   = "chmod 4755"                                ascii
        $gen    = "gen_group"                                 ascii
        $ko     = "libusb.ko"                                 ascii
        $mod    = "LoadModule remote_module"                  ascii
        $chattr = "chattr -i"                                 ascii
        $md5    = "e4045abc54e155f6bb430bbb77c9ff52"          ascii

        
        $act1 = "chmod 777 /etc/sudoers"                                      ascii
        $act2 = "sed -i '/LoadModule ssl_module/a LoadModule remote_module"   ascii
        $act3 = "/etc/modules-load.d/libusb.conf"                             ascii
        $act4 = "apachectl restart"                                           ascii

        
        $rm1 = "/LoadModule remote_module/d"                  ascii
        $rm2 = "rmmod libusb"                                 ascii
        $rm3 = "rm -f /etc/modules-load.d/libusb.conf"        ascii
        $rm4 = "tcp_early_demux=0"                            ascii

    condition:
        (uint16(0) == 0x2123)                 
        and not any of ($rm*)
        and (
            2 of ($act*)
            
            or (any of ($act*) and 2 of ($sudo, $suid, $gen, $ko, $mod, $chattr, $md5))
        )
        
}

