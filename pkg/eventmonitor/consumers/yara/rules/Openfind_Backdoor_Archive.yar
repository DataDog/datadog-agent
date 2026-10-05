/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Openfind_Backdoor_Archive
{
    meta:
        description = "Openfind MailGates backdoor delivery archive (tar containing escaping symlinks); family: QuietEnvelope"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "5df63139-b370-4f23-9b5d-357606317629"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a63a25883822123f91b363b90d69620e"
        sha256 = "e9b4c66bbdf4f9389af05cf1e1a605e57e8f6e7a7cf1ddb1e70435cffff83063"

    strings:
        
        $m1 = "18302/mgbnr.pl"                ascii
        $m2 = "18302/mg_audit_notify_mgr.pl"  ascii
        $m3 = "18302/mod_remote.so"           ascii
        $m4 = "18302/libusb.ko"               ascii
        $m5 = "18302/mg_eml2pdf"              ascii
        $m6 = "18302/gen_group"               ascii

        
        $esc = "../././../../util"            ascii

        
        $name = { 37 7A BC AF 27 1C }

        
        $usr = "webmail"                      ascii

    condition:
        
        uint32be(0) != 0x1f8b0800 and         
        for any i in (0..2) : (
            uint32(257 + i * 512) == 0x61747375   
        )
        and $esc
        and $name
        and $usr
        and 4 of ($m*)
}

