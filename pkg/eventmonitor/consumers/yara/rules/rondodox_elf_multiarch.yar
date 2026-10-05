rule rondodox_elf_multiarch
{
    meta:
        description               = "Detects RondoDox (Rondo) botnet ELF multi architecture variants"
        author                    = "Anish Bogati"
        date                      = "2025-12-08"

        yarahub_reference_md5     = "8735262237764f6bb3c233c8c987bf68"
        yarahub_uuid              = "d1cf7e9e-4f3c-4a9c-9f85-5f8e9c9b7b42"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

        malware_family            = "RondoDox"
        reference                 = "https://bazaar.abuse.ch/sample/3b02c502a23b26e4d76850cd524041ae16d282431f62a2c07564cf1c3d29a9d5/"

    strings:
        $email1  = "rondo2012@atomicmail.io" ascii
        $email2  = "bang2013@atomicmail.io" ascii
        $ua      = "User-Agent: rondo" ascii
        $ssh     = "SSH-2.0-MoTTY_Release_0.82" ascii
        $persist = "rondo:345:once:" ascii
        $cmd     = "qconnect0x0" ascii
        $init    = "# Provides:          rondo" ascii

    condition:
        3 of ($email1, $email2, $ua, $ssh, $persist, $cmd, $init)
}
