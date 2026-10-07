rule Linux_Downloader {
    meta:
        author = "@P4nd3m1cb0y"
        description = "Detects a Linux downloader targeting x64, x86, and arm64 architectures."
        date = "2024-08-18"
        reference = "https://x.com/Huntio/status/1823280152845107543"
        hash = "3fd87c6e3d681d7f7909902899e1bce6c5095cf5" // x86 version
        hash = "7b276653c3e09010c4ec0afe3f44859ec1f5d65d" // x64 version
        hash = "12cbba0f00dbf73ce66ed33e115dee2e9a25add2" // arm64 version
        
        yarahub_reference_md5 = "85d2c22b576a80d2d1da591b0d3a5d26"
        yarahub_uuid = "cc1c56e6-a083-41d2-bbee-9f62e8af37c3"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $s1 = "/usr/sbin/systemd" ascii
        $s2 = "./systemd" ascii
        $s3 = "[kworker/0:2]" ascii
        $arch1 = "l64" ascii
        $arch2 = "l32" ascii
        $arch3 = "a64" ascii 

        $sock_x64 = { BA 00 00 00 00 BE 01 00 00 00 BF 02 00 00 00 E8 ?? ?? ?? ?? }
        /*
            BA 00 00 00 00  mov     edx, IPPROTO_IP
            BE 01 00 00 00  mov     esi, SOCK_STREAM
            BF 02 00 00 00  mov     edi, AF_INET
            E8 ?? ?? ?? ??  call    socket
        */

        $sock_x86 = { 6A 00 6A 01 6A 02 E8 ?? ?? ?? }
        /*
            6A 00           push    0
            6A 01           push    1
            6A 02           push    2
            E8 ?? ?? ??     call    socket
        */

        $sock_arm64 = { 02 00 80 52 21 00 80 52 40 00 80 52 ?? 69 00 94 }
        /*
            02 00 80 52     mov        w2,#0x0
            21 00 80 52     mov        w1,#0x1
            40 00 80 52     mov        w0,#0x2
            ?? 69 00 94     bl         socket
        */

    condition:
        uint32(0) == 0x464C457F and
        filesize < 1MB and
        (3 of ($s*) and 1 of ($sock*) and 1 of ($arch*))
}
