rule win_lu0bot_loader_1d53 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2023-03-08"
        description               = "detects the loader of the Lu0bot malware"
        hash_md5                  = "c5eb9c6ded323a8db7eb739e514bb46c"
        hash_sha1                 = "cede3aa5e1821a47f416c64bc48d1aab72eb48ca"
        hash_sha256               = "5a2283a997ab6a9680b69f9318315df3c9e634b3c4dd4a46f8bc5df35fc81284"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "c5eb9c6ded323a8db7eb739e514bb46c"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "1d536a34-2111-40fe-aea8-d8e9062dfe8b"

    strings:
        /*
            add     edi, ?h
            sub     dword ptr [esi], <4 byte key>
            add     esi, 4
            (optional mov)
            cmp     esi, edi
        */
        $decryption = { 81 C7 ?? 0? 00 00
                        81 2E ?? ?? ?? ??
                        83 C6 04
                        [0-4]
                        39 FE}
        /*
            mov     ebx, 0
            push    ebx
            push    eax
            mov     eax, offset WinExec
            call    dword ptr [eax]
        */
        $winexec    = { BB 00 00 00 00
                        53
                        50
                        B8 ?? ?? ?? ??
                        FF 10}
        /*

            mov     eax, 0
            push    eax
            call    ExitProcess
        */
        $exit       = { B8 00 00 00 00
                        50
                        E8}

    condition:
        (uint16(0) == 0x5A4D) and
        $decryption and
        $winexec and
        $exit
}
