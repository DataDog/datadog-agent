/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Openfind_Backdoor_Injector
{
    meta:
        description = "mg_eml2pdf: ptrace injector that implants an SMTP covert channel into the mgsmtpd process"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "d99f855d-7636-45a6-8a1f-26624aca7f47"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "0a99a340ec802a88e8451c17767a5c0a"
        sha256 = "e7b46e13ebb0b1d4eed471f24d9ce9a7ab617c93b48b9316c10187776a7eb3cf"
        note = "Filename masquerades as the product's eml-to-pdf helper; the deployment script deletes it after execution."

    strings:
        
        $tgt = "/webmail/mqueue/bin/mgsmtpd"  ascii

        
        $f1 = "create_optimized_shellcode"    ascii
        $f2 = "remote_mmap_at"                ascii
        $f3 = "remote_mprotect_at"            ascii
        $f4 = "ptrace_write_attached"         ascii
        $f5 = "find_pids_by_exe"              ascii

        
        $cn1 = { e5 bc 80 e5 a7 8b e4 bf ae e6 94 b9 e6 9c ba e5 99 a8 e7 a0 81 e5 92 8c e6 b3 a8 e5 85 a5 20 73 68 65 6c 6c 63 6f 64 65 }
        $cn2 = { e4 bf ae e6 94 b9 e5 b7 b2 e5 ae 8c e6 88 90 e5 b9 b6 e6 b0 b8 e4 b9 85 e4 bf 9d e7 95 99 e5 9c a8 e5 86 85 e5 ad 98 e4 b8 ad }
        $cn3 = { e6 9c aa e6 89 be e5 88 b0 e7 89 b9 e5 be 81 e7 a0 81 }

    condition:
        uint32(0) == 0x464C457F
        and (
            $tgt
            or 3 of ($f*)
            or $cn1 or $cn2
            
            or ($cn3 and any of ($f*))
        )
}

