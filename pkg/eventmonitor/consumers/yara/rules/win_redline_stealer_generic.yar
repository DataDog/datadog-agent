rule win_redline_stealer_generic {
    meta:
        author = "dubfib"
        date = "2025-02-08"
        malpedia_family = "win.redline_stealer"

        yarahub_uuid = "c112ab5e-abbd-4736-a4eb-d9ec55120933"
        yarahub_reference_md5 = "86a35cbdccc3c59e9e0ffee1ff76dfbb"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_license = "CC BY 4.0"
        yarahub_reference_link = "https://github.com/dubfib/yara"

    strings:
        /* readable strings */
        $str0 = "*wallet*" wide ascii
        $str1 = "chromeKey" wide ascii
        $str2 = "waasflleasft.datasf" wide ascii 
        
        /* regex looking patterns */
        $str3 = "[AString-ZaString-z\\d]{2String4}\\.[String\\w-]{String6}\\.[\\wString-]{2String7}" wide ascii
        $str4 = "\\D*(\\d+)\\D*(\\d+)\\D*(\\d+)\\D*(\\d+)" wide ascii
        $str5 = "([a-zA-Z0-9]{1000,1500})" wide ascii
        
        /* random patterns */
        $str6 = "8A9E4EB40A426C455AEEA5205DE1CA71C9E3F2CB" wide ascii
        $str7 = "02C082D8E890E35EAD1E1F48955238C88F8E720A" wide ascii
        $str8 = "EB5B9A22EDFFCC85B7C7CC4E367A17C765EB1F94" wide ascii
        $str9 = "2B3013C9A195224A953A347DD5BE2F89E69AFC8D" wide ascii
        $str10 = "F51A29C4E21B24C17DDDBAFA4D63AADBBB8CEF56" wide ascii
        $str11 = "7AF697B6AE20B2C56159891E12E305799E817408" wide ascii
        $str12 = "4EFB3D976C53AD9818E202ED3B020AEE531E6608" wide ascii
        $str13 = "049E0C20203D3788799332A08CD378B400EE0675" wide ascii
        $str14 = "998C14ADF6EDA5504943349E0E29F88432F78017" wide ascii
        $str15 = "7245750688F8ABD9E50DA82005D421F8461EC1E6" wide ascii
        $str16 = "2FDC3DD404D36C2DDC0557974B6D14F030B61648" wide ascii
        $str17 = "99086C63443EF4224B60D2ED08447C082E7A0484" wide ascii
        $str18 = "37701C4DDBD6F4FFA34E2AC8D981116FCF0738F6" wide ascii
        $str19 = "AA89D91ACCC2516859C2335E0202A1D3F097B8EA" wide ascii
        $str20 = "44D48F7B4AC854FFD48261154223EED24DDBEA40" wide ascii
        $str21 = "792E9C91724FBC58C52DFE47FFA3209FE4AF82C5" wide ascii
        $str22 = "1C5A48F7FA44FCAB7E0A498A9601F8B8923CB9F2" wide ascii
        $str23 = "BCEF86DAFC99BA02019A51909C079A7A31931909" wide ascii
        $str24 = "35AB2B7BB0DD1BAAD55533B60B8B3720EBD2E662" wide ascii
        $str25 = "ADE0AE50AA98EA42836AF319D3F95FE04D6940B9" wide ascii
        $str26 = "0C4D52ADF2C2DD6611DB4F75CC71B8B16DE2C884" wide ascii
        $str27 = "EF96DD749202BF9E4A92A9B7DBAA2231D6954A85" wide ascii
        $str28 = "B0BC765D16360F75761A81546FB00C45CE3D3E86" wide ascii
        $str29 = "629E48D8C9F47770AEA997D814A074D7A1EF83BA" wide ascii
        $str30 = "9B2A039DF1F2AB2ADA2788B3286F7D1E28E4092A" wide ascii
        $str31 = "F0AE235AE8AF0108C208C56ADF7EA822F7D58E4F" wide ascii
        $str32 = "8C463F5736DA291784197E4FB70EA157E97F367F" wide ascii
        $str33 = "EA4E1BA587D831069E9D9270EF0F027BB829C8A5" wide ascii
        $str34 = "054958608D7C2571AB29F240B4EFD51B46277581" wide ascii
        $str35 = "D0684D3C4DDE072FACD943898351C8CEB561AAF7" wide ascii
        $str36 = "87C83ACF22482CDF8C6B13F4E661CBAE493B91BF" wide ascii
        $str37 = "5228E4D31C49B8491CE9A64B37F69147CCED17E1" wide ascii
        $str38 = "5D589FFC4215F1BB3ACEA6CE44CFCAF1548DD6DD" wide ascii

        /* random strings */
        $str39 = "Ly9zZXR0aW5nW0BuYW1lPSdQYXNzd29yZCddL3ZhbHVlconfig" wide ascii
        $str40 = "S#T']*^._/`2a?bOcReUfZg^hminnoorrs" wide ascii
        $str41 = "! #\",+0/54;:@?A?CBFEGEHEJILKMK{z|z}z~z" wide ascii
        $str42 = "L{41744BE4-11C5-494C-A213-BA0CE944938E" wide ascii
        $str43 = "ezB9XEZpHandlebGVaaWxsYVxyZWHandl" wide ascii
        $str44 = "c74790bd166600f1f665c8ce201776eb" wide ascii
        $str45 = "( (%(*(9(@(H(P(U(s(" wide ascii
        $str46 = "8!8'8-8<8N8Y8g8z8" wide ascii
        $str47 = "yF$$!$!!Se" wide ascii
        $str48 = ",l.qsE0+X:" wide ascii
        $str49 = "1*.1l1d1b" wide ascii
        $str50 = "&7'a'u'~'" wide ascii
        $str51 = "2!2&2L2]2" wide ascii
        $str52 = "#(#?#G#S#" wide ascii
        $str53 = "R	;9	|F&9" wide ascii
        $str54 = "*(+U+{+" wide ascii
        $str55 = "$:$B$J$" wide ascii

    condition:
        uint16(0) == 0x5a4d and
        3 of ($str*)
}