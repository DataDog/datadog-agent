rule DLL_TinyTurla_Strings_Feb2024 {
    meta:
        Description = "Detects Tiny Turla Implant used by Turla APT based on match strings"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Reference = "https://blog.talosintelligence.com/tinyturla-next-generation/"
        Hash = "267071df79927abd1e57f57106924dd8a68e1c4ed74e7b69403cdcdf6e6a453b"
        date = "2024-02-20"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "e4c356cf822cda0ca8e8161cb5bf6c39"
        yarahub_uuid = "e7680f16-a4f8-4d33-8d78-c95482385cba"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.tinyturla_ng"

    strings:
        $URLs1 = "https://thefinetreats.com/wp-content/themes/twentyseventeen/rss-old.php"
        $URLs2 = "https://hanagram.jp/wp/wp-content/themes/hanagram/rss-old.php"
        $URLsUnknown = /https:.{2,100}php/ //hardcoded PHP URLs in the samples

        $cmd1 = "changeshell"
        $cmd2 = "Set-PSReadLineOption -HistorySaveStyle SaveNothing"
        $cmd3 = "powershell.exe -nologo"
        $cmd4 = "chcp 437 > $null"
        $cmd5 = "reg dexplorer.exe"
        $cmd6 = "delkill /F /IM explENT_USER"
        $cmd7 = "if exist {C2796011-81BA-4148-8FCA-C664324elete"
        $cmd8 = "task\"%s\" goto d"

        $usragnt1 = "Mozilla/5.0 (Windows NT 6.1"
        $usragnt2 = "rv:2.0.1) Gecko/20100101 Firefox/4.0.1"
        $usragnt3 = "Content-Disposition: form-data"
        $usragnt4 = "name=\"gettask\""
        $usragnt5 = "name=\"id\""
        $usragnt6 = "name=\"file\""
        $usragnt7 = "Content-Type: application/octet-stream"
        $usragnt8 = "filename=\"%s\""

    condition:
        any of ($URLs*)
        and 4 of ($cmd*)
        and 5 of ($usragnt*)
       
 }

// Day51.yar has the YARA rule based on the PE imports and exports