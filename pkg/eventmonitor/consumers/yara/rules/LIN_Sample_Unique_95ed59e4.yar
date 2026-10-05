rule LIN_Sample_Unique_95ed59e4 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "95ed59e4218629b99427848fd1a160b5a1f2dfde4b43494bb4136b29d6a7311f.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "7ea612ea-0152-431e-96b5-3a2fb987d5cb"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "e3ed5b9a446bf8c3ec682e966351df84"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "		sed -i 's/mirrors.cloud.aliyuncs.com/url_tmp/g'  /etc/yum.repos.d/Centos-vault-8.5.2111.repo &&  sed -i 's/mirrors.aliyun.com/mirrors.cloud.aliyuncs.com/g' /etc/yum.repos.d/Centos-vault-8.5.2111.repo && sed -i 's/url_tmp/mirrors.aliyun.com/g' /etc/yum.repos.d/Centos-vault-8.5.2111.repo" ascii wide
        $b = "	pacman -S curl wget unzip firewalld openssl pkg-config make gcc cmake libxml2 libxslt libvpx gd libsodium oniguruma sqlite libzip autoconf inetutils sudo --noconfirm" ascii wide
    condition:
        all of them
}
