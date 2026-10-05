rule win_dotnet_vb_stealer_rat
{
    meta:
        author                      = "Arrbat"
        date                        = "2026-04-30"
        family                      = "win.dotnet_vb_rat"
        description                 = "Detects .NET/VB executables with stealer/RAT capabilities (sockets, HTTP, processes, registry, etc)."
        yarahub_author_github       = "Arrbat"
        yarahub_reference_md5       = "875f2a3ea70058fa8ab3cedecc5c2745"
        yarahub_reference_md5_2     = "5cd5e8ef66e625e34a28b00720698dd7"
        yarahub_uuid                = "9bdbfd5c-167b-424a-a977-29bd554e8175"
        yarahub_license             = "CC0 1.0"
        yarahub_rule_matching_tlp   = "TLP:WHITE"
        yarahub_rule_sharing_tlp    = "TLP:WHITE"

    strings:
        $vb_1 = "My.Computer"                 ascii wide
        $vb_2 = "My.Application"              ascii wide
        $vb_3 = "My.Settings"                 ascii wide
        $vb_4 = "Microsoft.VisualBasic"       ascii wide

        /* RAT/Exfiltration indicators */
        $webclient = "System.Net.WebClient"         ascii wide  
        $ftp       = "System.Net.FtpWebRequest"     ascii wide
        $tcp       = "System.Net.Sockets.TcpClient" ascii wide 
        $udp       = "System.Net.Sockets.UdpClient" ascii wide
        $socket    = "System.Net.Sockets.Socket"    ascii wide
        $process   = "System.Diagnostics.Process"   ascii wide 
        $openkey   = "OpenSubKey"                   ascii       
        $netCred   = "NetworkCredential"            ascii wide  
        $rij       = "RijndaelManaged"              ascii wide  /* AES crypto */
        $dpapi     = "ProtectedData"                ascii wide 
    condition:
        (uint16(0) == 0x5A4D) and
        (2 of ($vb_1, $vb_2, $vb_3, $vb_4)) and
        (2 of ($webclient, $ftp, $tcp, $udp, $socket, $process, $openkey, $netCred, $rij, $dpapi))
}

