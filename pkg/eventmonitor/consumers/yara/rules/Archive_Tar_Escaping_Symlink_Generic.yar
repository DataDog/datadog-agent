/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Archive_Tar_Escaping_Symlink_Generic
{
    meta:
        description = "Generic: tar archive with chained escaping symlinks - symlink A points shallower than itself, symlink B's target traverses A and climbs further up"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "0c511dc7-f3df-481a-9478-089423ffccdb"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a63a25883822123f91b363b90d69620e"
        note = "Parses tar headers rather than matching literal ../ strings. Only the first 256 blocks are sampled: a large decoy file can push the chain out of that window. Trade-off, not a structural limit."

    condition:
        uint32(257) == 0x61747375                     

        and for any i in (0..255) : (                 
            uint32(257 + i * 512) == 0x61747375
            and uint8(156 + i * 512) == 0x32          
            and (
                uint8(157 + i * 512) == 0x2F          
                or ( uint8(157 + i * 512) == 0x2E and uint8(158 + i * 512) == 0x2E )   
            )
            and uint8(i * 512 + 2) != 0               

            and for any j in (0..255) : (             
                uint32(257 + j * 512) == 0x61747375
                and uint8(156 + j * 512) == 0x32
                and j != i

                
                and for all k in (0..47) : (
                    uint8(i*512 + k) == 0
                    or uint8(157 + j*512 + k) == uint8(i*512 + k)
                )
                
                and for any m in (3..47) : (
                    uint8(i*512 + m) == 0 and uint8(157 + j*512 + m) == 0x2F
                )

                
                and for any k in (0..97) : (
                    uint8(157 + j*512 + k) == 0x2E and uint8(158 + j*512 + k) == 0x2E
                )
            )
        )
}

