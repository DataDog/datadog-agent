rule LNK_Dropper_Russian_APT_Feb2024 {
    meta:
        Description = "Detects LNK dropper samples used by a Russian APT during a past campaign"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Reference = "https://blog.cluster25.duskrise.com/2024/01/30/russian-apt-opposition"
        Hash = "114935488cc5f5d1664dbc4c305d97a7d356b0f6d823e282978792045f1c7ddb"
        SampleTesting = "Matches all five LNK Dropper Samples from the Blog"
        date = "2024-02-05"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "b4f10039927b040f0470b956c74a31b4"
        yarahub_uuid = "569aa505-3cb6-4747-bbe6-38e6756ebd6e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $lnk = { 4C 00 00 00 01 14 02 00 }
        $pwrsh1 = "powershell.exe"
        $pwrsh2 = "WindowsPowerShell"
        $pwrsh3 = "powershell"
        $cmd = "cmd.exe"
        $ext1 = ".pdf.lnk" 
        $ext2 = ".pdfx.lnk"
        $ext3 = "pdf.lnk" base64
        $scrpt1 = "Select-String -pattern \"JEVycm9yQWN0aW9uUH\" "
        $scrpt2 = "findstr /R 'JVBERi0xLjcNJeLjz9'" base64
        $blob1 = "$ErrorActionPreference = \"Continue\"" base64
        $blob2 = "$ProgressPreference = \"SilentlyContinue\"" base64
        $blob3 = "New-Alias -name pwn -Value iex -Force" base64
        $blob4 = "if ($pwd.path.toLower() -ne \"c:\\windows\\system32\")" base64
        $blob5 = "Copy-Item $env:tmp\\Temp.jpg $env:userprofile\\Temp.jpg" base64
        $blob6 = "attrib +h $env:userprofile\\Temp.jpg" base64
        $blob7 = "Start-Process $env:tmp\\Important.pdf" base64
        $net1 = "$userAgent = \"Mozilla/6.4 (Windows NT 11.1) Gecko/2010102 Firefox/99.0\"" base64
        $net2 = "$redirectors = \"6" base64
        $net3 = "$sleeps = 5" base64
        $http1 = "$request.Headers[\"X-Request-ID\"] = $request_token" base64
        $http2 = "$request.ContentType = \"application/x-www-form-urlencoded\"" base64
        $http3 = "$response1 = $(Send-HttpRequest \"$server/api/v1/Client/Info\" \"POST\" \"Info: $getenv64\")" base64
        $http4 = "$response = $($token = Send-HttpRequest \"$server/api/v1/Client/Token\" \"GET\")" base64
        $server1 = "$server = \"api-gate.xyz\"" base64
        $server2 = "$server = \"pdf-online.top\"" base64
        $unknown = "$server = " base64
        
    condition:
        $lnk at 0                       //LNK File Header
        and (any of ($pwrsh*) or $cmd) //searches for CMD or PowerShell execution 
        and any of ($ext*)            //Fake Double Extension mimicing a PDF
        and any of ($scrpt*)         //Searches for a unique string to locate execution code
        and 5 of ($blob*)           //Base64 encoded execution blob
        and 2 of ($net*) 
        and 3 of ($http*)
        and (any of ($server*) or $unknown) // C2 dommain config (Optional, can be removed)
        
 }

