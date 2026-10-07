/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Openfind_Backdoor_TriggerScript
{
    meta:
        description = "mg_audit_notify_mgr.pl: trojanised trigger script that runs mgbnr.pl as root through mg_sudo"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "4c572584-0178-4eba-b698-3922f122f772"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "1d22b5f39d1d2ff57e7b69f63139252b"
        sha256 = "30417588c1e007ac7a5f08fb11e580ae7c3548b5d67e498bc12d869a8d38dffe"
        note = "318 bytes total: the vendor copyright header plus a single malicious call. The vendor original calls mg_audit_notify, never mgbnr.pl."

    strings:
        
        $call = "mg_sudo /mailgates/mg/util/mgbnr.pl"        ascii

        
        $a1 = "mg_sudo"                                       ascii
        $a2 = "mgbnr.pl"                                      ascii

        
        $hdr = "Openfind Information Technology"              ascii
        $desc = "invoking mg_audit_notify"                    ascii

    condition:
        uint16(0) == 0x2123                    
        and filesize < 4KB
        and (
            $call
            or (all of ($a*) and $hdr)
            or ($desc and $a2)                 
        )
}
