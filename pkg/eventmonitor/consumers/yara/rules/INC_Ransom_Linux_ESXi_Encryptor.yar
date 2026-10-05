rule INC_Ransom_Linux_ESXi_Encryptor
{
    meta:
        description                = "Detects the INC Ransom Linux/VMware ESXi encryptor (Rust; X25519 + AES-128-CTR / Salsa20)"
        author                     = "Peter Gabaldon"
        date                       = "2026-09-18"
        yarahub_uuid               = "38deae5f-3967-484f-888f-a8374b0442a7"
        yarahub_license            = "CC0 1.0"
        yarahub_rule_matching_tlp  = "TLP:WHITE"
        yarahub_rule_sharing_tlp   = "TLP:WHITE"
        yarahub_reference_md5      = "04bafa07c24c7dd08738bee51e43956c"

    strings:
        $note    = "INC-README.txt" ascii
        $b64note = "fn5+fiBJTkMgUmFuc29t" ascii            // base64("~~~~ INC Ransom")

        $onion1  = "incblog6qu4y4mm4zvw5nrmue6qbwtgjsxpw6b7ixzssu36tsajldoad" ascii
        $onion2  = "incpaykabjqc2mtdxq6c23nqh4x6m5dkps5fr6vgdkgzp5njssx6qkid" ascii

        $cli1    = "Encryption mode (fast, medium, slow)" ascii
        $cli2    = "Replace default Message of the day by ransom note" ascii
        $cli3    = "Detach an application from SSH connection, so you can close it and continue encryption" ascii
        $cli4    = "Skip virtual machines with given IDs separated by ','" ascii

        $err1    = "' while stopping ESXi machines" ascii
        $err2    = "' while removing ESXi snapshots" ascii
        $err3    = "Failed to get block size" ascii

        $esxi1   = "vmsvc/power.off" ascii
        $esxi2   = "vmsvc/snapshot.removeall" ascii

        $crate1  = "x25519_dalek" ascii
        $crate2  = "aes-soft-0.6.4" ascii
        $crate3  = "salsa20-0.9.0" ascii

    condition:
        uint32(0) == 0x464c457f and filesize < 20MB and
        (
            ( $b64note and $note ) or
            ( 2 of ( $onion* ) ) or
            ( 3 of ( $cli* ) ) or
            ( 2 of ( $err* ) and 1 of ( $esxi* ) ) or
            ( all of ( $crate* ) and $note )
        )
}
