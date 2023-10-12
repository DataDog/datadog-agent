// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the entrypoint of the compliance k8s_types_generator tool
// that is responsible for generating various configuration types of
// Kubernetes components.
package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/exp/slices"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	bindir = ""
	arch   = ""

	k8sComponents = []string{"kube-apiserver", "kube-scheduler", "kube-controller-manager", "kube-proxy", "kubelet"}

	// https://kubernetes.io/releases/
	k8sVersions = []string{
		"v1.27.3",
		"v1.26.6",
		"v1.25.11",
		"v1.24.15",
		"v1.23.17",
	}

	// https://github.com/kubernetes/kubernetes/blob/c3e7eca7fd38454200819b60e58144d5727f1bbc/cluster/images/etcd/Makefile#L18
	// "v3.0.17", "v3.1.20" removed because they do not have ARM64 tarballs
	etcdVersions = []string{
		"v3.5.7",
		"v3.4.18",
		"v3.3.17",
		"v3.2.32",
	}

	knownFlags = []string{
		"--address",
		"--admission-control-config-file",
		"--allow-privileged",
		"--anonymous-auth",
		"--audit-log-maxage",
		"--audit-log-maxbackup",
		"--audit-log-maxsize",
		"--audit-log-path",
		"--audit-policy-file",
		"--authentication-kubeconfig",
		"--authorization-kubeconfig",
		"--authorization-mode",
		"--auto-tls",
		"--bind-address",
		"--cert-file",
		"--client-ca-file",
		"--client-cert-auth",
		"--cluster-signing-cert-file",
		"--cluster-signing-key-file",
		"--config",
		"--data-dir",
		"--disable-admission-plugins",
		"--enable-admission-plugins",
		"--enable-bootstrap-token-auth",
		"--encryption-provider-config",
		"--etcd-cafile",
		"--etcd-certfile",
		"--etcd-keyfile",
		"--event-burst",
		"--event-qps",
		"--feature-gates",
		"--hostname-override",
		"--image-credential-provider-bin-dir",
		"--image-credential-provider-config",
		"--key-file",
		"--kubeconfig",
		"--kubelet-certificate-authority",
		"--kubelet-client-certificate",
		"--kubelet-client-key",
		"--make-iptables-util-chains",
		"--max-pods",
		"--peer-auto-tls",
		"--peer-cert-file",
		"--peer-client-cert-auth",
		"--peer-key-file",
		"--peer-trusted-ca-file",
		"--pod-max-pids",
		"--profiling",
		"--protect-kernel-defaults",
		"--proxy-client-cert-file",
		"--proxy-client-key-file",
		"--read-only-port",
		"--request-timeout",
		"--requestheader-allowed-names",
		"--requestheader-client-ca-file",
		"--requestheader-extra-headers-prefix",
		"--requestheader-group-headers",
		"--requestheader-username-headers",
		"--root-ca-file",
		"--rotate-certificates",
		"--rotate-server-certificates",
		"--secure-port",
		"--service-account-issuer",
		"--service-account-key-file",
		"--service-account-lookup",
		"--service-account-private-key-file",
		"--service-account-signing-key-file",
		"--service-cluster-ip-range",
		"--streaming-connection-idle-timeout",
		"--terminated-pod-gc-threshold",
		"--tls-cert-file",
		"--tls-cipher-suites",
		"--tls-private-key-file",
		"--token-auth-file",
		"--trusted-ca-file",
		"--use-service-account-credentials",
	}
)

const preamble = `// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// !!!
// This is a generated file: regenerate with go run ./pkg/compliance/tools/k8s_types_generator.go
// !!!
//revive:disable
package k8sconfig

import (
	"time"
	"strings"
)
`

type conf struct {
	versions    []string
	flagName    string
	flagType    string
	flagDefault string
	goType      string
}

type komponent struct {
	name    string
	version string
	confs   []*conf
}

// go run ./pkg/compliance/tools/k8s_types_generator/main.go ./pkg/compliance/tools/bin | gofmt > ./pkg/compliance/k8sconfig/types_generated.go
func main() {
	dir, _ := os.Getwd()
	if len(os.Args) < 2 {
		fmt.Println("generator <bindir>")
		log.Fatal("missing bindir path")
	}
	bindir = filepath.Join(dir, os.Args[1])
	if info, err := os.Stat(bindir); err != nil || !info.IsDir() {
		log.Fatalf("bindir path %s is not a directory", bindir)
	}
	uname, _ := exec.Command("uname", "-m").Output()
	switch string(bytes.TrimSuffix(uname, []byte("\n"))) {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	default:
		log.Fatalf("could not resolve arch=%s", uname)
	}

	fmt.Print(preamble)
	var allKomponents []*komponent
	for _, component := range k8sComponents {
		var komponents []*komponent
		for _, version := range k8sVersions {
			komp := downloadKubeComponentAndExtractFlags(component, version)
			komponents = append(komponents, komp)
		}
		mergedKomp := unionKomponents(komponents...)
		allKomponents = append(allKomponents, mergedKomp)
		fmt.Println(printKomponentCode(mergedKomp))
	}

	{
		var komponents []*komponent
		for _, version := range etcdVersions {
			komp := downloadEtcdAndExtractFlags(version)
			komponents = append(komponents, komp)
		}
		mergedKomp := unionKomponents(komponents...)
		allKomponents = append(allKomponents, mergedKomp)
		fmt.Println(printKomponentCode(mergedKomp))
	}

	var knownFlagsClone []string
	knownFlagsClone = append(knownFlagsClone, knownFlags...)
	for _, komponent := range allKomponents {
		for _, conf := range komponent.confs {
			i := slices.Index(knownFlagsClone, "--"+conf.flagName)
			if i >= 0 {
				knownFlagsClone = append(knownFlagsClone[:i], knownFlagsClone[i+1:]...)
			}
		}
	}
	if len(knownFlagsClone) > 0 {
		panic(fmt.Errorf("these flags were not found: %v", knownFlagsClone))
	}
}

func defaultedType(componentName string, conf *conf) *conf {
	if conf.flagName == "kubeconfig" || conf.flagName == "authentication-kubeconfig" {
		conf.flagType = "kubeconfig"
	} else if conf.flagType == "string" || conf.flagType == "stringArray" {
		switch {
		case strings.Contains(conf.flagName, "cert"),
			strings.Contains(conf.flagName, "cafile"),
			strings.Contains(conf.flagName, "ca-file"):
			conf.flagType = "certificate_file"
		case strings.HasSuffix(conf.flagName, "keyfile"),
			strings.HasSuffix(conf.flagName, "key-file"),
			strings.HasSuffix(conf.flagName, "key"):
			conf.flagType = "key_file"
		case strings.Contains(conf.flagName, "token"):
			conf.flagType = "token_file"
		case conf.flagName == "encryption-provider-config":
			conf.flagType = "encryption_config_file"
		case conf.flagName == "admission-control-config-file":
			conf.flagType = "admission_config_file"
		case conf.flagName == "config", conf.flagName == "audit-policy-file", conf.flagName == "image-credential-provider-config":
			conf.flagType = "config_file"
		case strings.Contains(conf.flagName, "dir"):
			conf.flagType = "dir"
		}
	}

	switch conf.flagType {
	case "bool":
		conf.flagDefault = parseTypeBool(conf.flagDefault)
		conf.goType = "bool"
	case "cidrs":
		conf.flagDefault = parseTypeCIDRs(conf.flagDefault)
		conf.goType = "string"
	case "duration":
		conf.flagDefault = parseTypeDuration(conf.flagDefault)
		conf.goType = "time.Duration"
	case "float", "float32":
		conf.flagDefault = parseTypeFloat(conf.flagDefault)
		conf.goType = "float64"
	case "int", "int32", "quantity", "uint":
		conf.flagDefault = parseTypeNumber(conf.flagDefault)
		conf.goType = "int"
	case "ip", "ipport":
		conf.flagDefault = parseTypeIP(conf.flagDefault)
		conf.goType = "string"
	case "mapStringBool":
		conf.flagDefault = parseEmptyDefault(conf.flagDefault)
		conf.goType = "string"
	case "mapStringString":
		conf.flagDefault = parseEmptyDefault(conf.flagDefault)
		conf.goType = "string"
	case "namedCertKey":
		conf.flagDefault = parseEmptyDefault(conf.flagDefault)
		conf.goType = "string"
	case "portRange":
		conf.flagDefault = parseTypeRange(conf.flagDefault)
		conf.goType = "string"
	case "severity":
		conf.flagDefault = parseTypeNumber(conf.flagDefault)
		conf.goType = "int"
	case "string",
		"LocalMode", "ProxyMode", "RuntimeDefault": // https://kubernetes.io/docs/reference/config-api/kube-proxy-config.v1alpha1/#kubeproxy-config-k8s-io-v1alpha1-LocalMode
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "string"
	case "kubeconfig":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sKubeconfigMeta"
	case "certificate_file":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sCertFileMeta"
	case "key_file":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sKeyFileMeta"
	case "config_file":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sConfigFileMeta"
	case "admission_config_file":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sAdmissionConfigFileMeta"
	case "encryption_config_file":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sEncryptionProviderConfigFileMeta"
	case "token_file":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sTokenFileMeta"
	case "dir":
		conf.flagDefault = parseTypeString(conf.flagDefault)
		conf.goType = "*K8sDirMeta"
	case "strings", "moduleSpec":
		conf.flagDefault = parseTypeStringsArray(conf.flagDefault)
		conf.goType = "[]string"
	case "stringToString", "stringArray":
		conf.flagDefault = parseEmptyDefault(conf.flagDefault)
		conf.goType = "string"
	case "traceLocation":
		conf.flagDefault = parseEmptyDefault(conf.flagDefault)
		conf.goType = "string"
	case "colonSeparatedMultimapStringString":
		// k8s.io/component-base/cli/flag
		conf.flagDefault = parseEmptyDefault(conf.flagDefault)
		conf.goType = "string"
	}
	if conf.flagDefault == "${name}.etcd" {
		conf.flagDefault = ""
	}
	if conf.goType == "" {
		log.Fatalf("unknown type for flag %q: %s (%q)", conf.flagName, conf.flagType, conf.flagDefault)
	}
	return conf
}

func unionKomponents(ks ...*komponent) *komponent {
	var confs []*conf
	for _, k := range ks {
		for _, newConf := range k.confs {
			var conf *conf
			for _, c := range confs {
				if c.flagName == newConf.flagName {
					conf = c
				}
			}
			if conf == nil {
				confs = append(confs, newConf)
				conf = newConf
			} else {
				if conf.flagType != newConf.flagType {
					panic("TODO: different types across versions")
				}
			}
			conf.versions = append(conf.versions, k.version)
		}
	}
	sort.Slice(confs, func(i, j int) bool {
		return strings.Compare(confs[i].flagName, confs[j].flagName) < 0
	})
	return &komponent{
		name:    ks[0].name,
		version: ks[0].version,
		confs:   confs,
	}
}

func printKomponentCode(komp *komponent) string {
	printAssignment := func(c *conf, v string) string {
		switch c.goType {
		case "string", "ip":
			return fmt.Sprintf("res.%s = %s", toGoField(c.flagName), v)
		case "bool":
			return fmt.Sprintf("res.%s = l.parseBool(%s)", toGoField(c.flagName), v)
		case "float64":
			return fmt.Sprintf("res.%s = l.parseFloat(%s, 64)", toGoField(c.flagName), v)
		case "int", "uint":
			return fmt.Sprintf("res.%s = l.parseInt(%s)", toGoField(c.flagName), v)
		case "time.Duration":
			return fmt.Sprintf("res.%s = l.parseDuration(%s)", toGoField(c.flagName), v)
		case "[]string":
			return fmt.Sprintf("res.%s = strings.Split(%s, \",\")", toGoField(c.flagName), v)
		case "*K8sKubeconfigMeta":
			return fmt.Sprintf("res.%s = l.loadKubeconfigMeta(%s)", toGoField(c.flagName), v)
		case "*K8sCertFileMeta":
			return fmt.Sprintf("res.%s = l.loadCertFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sKeyFileMeta":
			return fmt.Sprintf("res.%s = l.loadKeyFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sTokenFileMeta":
			return fmt.Sprintf("res.%s = l.loadTokenFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sConfigFileMeta":
			if komp.name == "kubelet" && c.flagName == "config" {
				return fmt.Sprintf("res.%s = l.loadKubeletConfigFileMeta(%s)", toGoField(c.flagName), v)
			}
			return fmt.Sprintf("res.%s = l.loadConfigFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sKubeletConfigFileMeta":
			return fmt.Sprintf("res.%s = l.loadKubeletConfigFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sAdmissionConfigFileMeta":
			return fmt.Sprintf("res.%s = l.loadAdmissionConfigFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sEncryptionProviderConfigFileMeta":
			return fmt.Sprintf("res.%s = l.loadEncryptionProviderConfigFileMeta(%s)", toGoField(c.flagName), v)
		case "*K8sDirMeta":
			return fmt.Sprintf("res.%s = l.loadDirMeta(%s)", toGoField(c.flagName), v)
		default:
			panic(fmt.Errorf("non supported type %s %s %s %q for with default %s", komp.name, komp.version, c.flagName, c.goType, c.flagDefault))
		}
	}

	titled := cases.Title(language.English, cases.NoLower).String(komp.name)
	goStructName := strings.ReplaceAll(titled, "-", "")
	s := ""
	s += fmt.Sprintf("type K8s%sConfig struct {\n", goStructName)
	for _, c := range komp.confs {
		if !isKnownFlag(c.flagName) {
			continue
		}
		s += fmt.Sprintf(" %s %s `json:\"%s\"` // versions: %s\n",
			toGoField(c.flagName), c.goType, toGoJSONTag(c.flagName), strings.Join(c.versions, ", "))
	}
	s += " SkippedFlags map[string]string `json:\"skippedFlags,omitempty\"`\n"
	s += "}\n"
	s += fmt.Sprintf("func (l *loader) newK8s%sConfig(flags map[string]string) *K8s%sConfig {\n", goStructName, goStructName)
	s += "if (flags == nil) { return nil }\n"
	s += fmt.Sprintf("var res K8s%sConfig\n", goStructName)
	for _, c := range komp.confs {
		if !isKnownFlag(c.flagName) {
			continue
		}
		s += fmt.Sprintf("if v, ok := flags[\"--%s\"]; ok {\n", c.flagName)
		s += fmt.Sprintf("delete(flags, \"--%s\")\n", c.flagName)
		s += printAssignment(c, "v")
		if c.flagDefault != "" {
			s += "\n} else {\n"
			s += printAssignment(c, fmt.Sprintf("%q", c.flagDefault))
		}
		s += "}\n"
	}
	s += "if len(flags) > 0 { res.SkippedFlags = flags }\n"
	s += "return &res\n"
	s += "}\n"
	return s
}

func downloadEtcdAndExtractFlags(componentVersion string) *komponent {
	const componentName = "etcd"
	componentBin := path.Join(bindir, fmt.Sprintf("%s-%s", componentName, componentVersion))
	componentTar := path.Join(bindir, fmt.Sprintf("%s-%s.tar.gz", componentName, componentVersion))
	componentURL := fmt.Sprintf("https://github.com/etcd-io/etcd/releases/download/%s/etcd-%s-linux-%s.tar.gz",
		componentVersion, componentVersion, arch)
	if _, err := os.Stat(componentBin); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "downloading %s into %s...", componentURL, componentBin)
		if err := download(componentURL, componentTar); err != nil {
			log.Fatal(err)
		}
		t, err := os.Open(componentTar)
		if err != nil {
			log.Fatal(err)
		}
		defer t.Close()
		g, err := gzip.NewReader(t)
		if err != nil {
			log.Fatal(err)
		}
		r := tar.NewReader(g)
		for {
			header, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, "/etcd") {
				outFile, err := os.Create(componentBin)
				if err != nil {
					log.Fatal(err)
				}
				if _, err := io.Copy(outFile, r); err != nil {
					log.Fatal(err)
				}
				outFile.Close()
			}

		}
		fmt.Fprintf(os.Stderr, "ok\n")
	}

	if err := os.Chmod(componentBin, 0770); err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command(componentBin, "-h")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "ETCD_UNSUPPORTED_ARCH=arm64")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	var confs []*conf
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		conf, ok := scanEtcdHelpLine(line)
		if ok {
			confs = append(confs, defaultedType(componentName, conf))
		}
	}
	return &komponent{
		name:    componentName,
		version: componentVersion,
		confs:   confs,
	}
}

func downloadKubeComponentAndExtractFlags(componentName, componentVersion string) *komponent {
	componentBin := path.Join(bindir, fmt.Sprintf("%s-%s", componentName, componentVersion))
	componentURL := fmt.Sprintf("https://dl.k8s.io/%s/bin/linux/%s/%s",
		componentVersion, arch, componentName)
	if _, err := os.Stat(componentBin); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "downloading %s into %s...", componentURL, componentBin)
		if err := download(componentURL, componentBin); err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "ok\n")
	}

	if err := os.Chmod(componentBin, 0770); err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command(componentBin, "-h")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	var confs []*conf
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		conf, ok := scanK8sHelpLine(line)
		if ok {
			confs = append(confs, defaultedType(componentName, conf))
		}
	}

	return &komponent{
		name:    componentName,
		version: componentVersion,
		confs:   confs,
	}
}

func toGoField(s string) string {
	caser := cases.Title(language.English, cases.NoLower)
	return strings.ReplaceAll(caser.String(s), "-", "")
}

func toGoJSONTag(s string) string {
	return s
}

func scanEtcdHelpLine(line string) (*conf, bool) {
	var conf conf
	var ok bool

	str := eatWhitespace(line)
	conf.flagName, str, ok = eatRegexp(str, "--([a-zA-Z0-9-]+)", 1)
	if !ok {
		return nil, false
	}
	if conf.flagName == "log-rotation-config-json" { // json flag
		return nil, false
	}
	if strings.HasPrefix(conf.flagName, "experimental-") {
		return nil, false
	}

	str = eatWhitespace(str)
	if str == "" {
		conf.flagDefault = "false"
		conf.flagType = "bool"
		return &conf, true
	}

	conf.flagDefault, str, ok = eatRegexp(str, "'(true|false)'", 1)
	if ok {
		conf.flagType = "bool"
		conf.goType = "bool"
		return &conf, true
	}
	conf.flagDefault, str, ok = eatRegexp(str, "([0-9]+)", 1)
	if ok {
		conf.flagType = "int"
		conf.goType = "int"
		return &conf, true
	}
	conf.flagDefault, _, ok = eatRegexp(str, "'(\\S*)'", 1)
	if ok {
		conf.flagType = "string"
		conf.goType = "string"
		return &conf, true
	}

	log.Fatalf("could not flag line: %s", line)
	return nil, false
}

func scanK8sHelpLine(line string) (*conf, bool) {
	var conf conf
	var ok bool

	str := eatWhitespace(line)
	conf.flagName, str, ok = eatRegexp(str, "--([a-zA-Z0-9-]+)", 1)
	if !ok {
		return nil, false
	}

	str = eatWhitespace(str)
	conf.flagType, str, ok = eatRegexp(str, "([a-zA-Z0-9]+)[ ]{3,}", 1)
	if ok {
		str = eatWhitespace(str)
	}

	if idx := strings.Index(str, "[default="); idx >= 0 {
		conf.flagDefault = scanDefaultValue(str[idx+len("[default="):], '[', ']')
	} else if idx := strings.Index(str, "[default "); idx >= 0 {
		conf.flagDefault = scanDefaultValue(str[idx+len("[default "):], '[', ']')
	} else if idx := strings.Index(str, "(default "); idx >= 0 {
		conf.flagDefault = scanDefaultValue(str[idx+len("(default "):], '(', ')')
	}
	if conf.flagType == "" {
		conf.flagType = "bool"
	}
	return &conf, true
}

func scanDefaultValue(str string, op, cl rune) string {
	var length int
	balance := 1
	for _, r := range str {
		if r == op {
			balance++
		} else if r == cl {
			balance--
		}
		length++
		if balance == 0 {
			break
		}
	}
	val := str[:length]
	val = strings.TrimPrefix(val, string(op)+"default")
	val = strings.TrimSuffix(val, string(cl))
	return strings.TrimSpace(val)
}

func parseTypeBool(str string) string {
	if str == "" {
		str = "false"
	}
	b, err := strconv.ParseBool(str)
	if err != nil {
		log.Fatal(err)
	}
	if b {
		return "true"
	}
	return ""
}

func parseTypeCIDRs(str string) string {
	var cidrs []string
	for _, s := range strings.Split(str, ",") {
		s = strings.TrimSpace(s)
		_, _, err := net.ParseCIDR(s)
		if err != nil {
			log.Fatal(err)
		}
		cidrs = append(cidrs, s)
	}
	return strings.Join(cidrs, ",")
}

func parseTypeDuration(str string) string {
	if str == "" {
		str = "0"
	}
	_, err := time.ParseDuration(str)
	if err != nil {
		log.Fatal(err)
	}
	return str
}

func parseTypeFloat(str string) string {
	if str == "" {
		str = "0.0"
	}
	_, err := strconv.ParseFloat(str, 64)
	if err != nil {
		log.Fatal(err)
	}
	return str
}

func parseTypeNumber(str string) string {
	if str == "" {
		str = "0"
	}
	_, err := strconv.Atoi(str)
	if err != nil {
		log.Fatal(err)
	}
	return str
}

func parseTypeIP(str string) string {
	ip := net.ParseIP(str)
	return ip.String()
}

func parseTypeRange(str string) string {
	r := regexp.MustCompile("^[0-9]+-[0-9]+$")
	if !r.MatchString(str) {
		log.Fatalf("bad range type default %q", str)
	}
	return str
}

func parseTypeString(str string) string {
	if strings.HasPrefix(str, "\"") {
		if !strings.HasSuffix(str, "\"") {
			log.Fatalf("bad string type default %q", str)
		}
		return str[1 : len(str)-1]
	}
	if strings.HasPrefix(str, "'") {
		if !strings.HasSuffix(str, "'") {
			log.Fatalf("bad string type default %q", str)
		}
		return str[1 : len(str)-1]
	}
	return str
}

func parseTypeStringsArray(str string) string {
	if strings.HasPrefix(str, "[") {
		if !strings.HasSuffix(str, "]") {
			log.Fatalf("bad string type default %q", str)
		}
		str = str[1 : len(str)-1]
	}
	strs := strings.Split(str, ",")
	for i, str := range strs {
		strs[i] = strings.TrimSpace(str)
	}
	return strings.Join(strs, ",")
}

func parseEmptyDefault(str string) string {
	if str == "imagefs.available<15%,memory.available<100Mi,nodefs.available<10%,nodefs.inodesFree<5%" {
		// special case for deprecated flag with said default kubelet --eviction-hard
		return ""
	}
	if str != "" && str != "[]" && str != "none" && str != ":0" {
		log.Fatalf("bad empty type default %q", str)
	}
	return ""
}

func eatWhitespace(str string) string {
	i := 0
	for _, r := range str {
		if !unicode.IsSpace(r) {
			break
		}
		i++
	}
	return str[i:]
}

func eatRegexp(str, reg string, group int) (string, string, bool) {
	r := regexp.MustCompile("^" + reg)
	loc := r.FindStringSubmatchIndex(str)
	if loc != nil {
		if loc[0] != 0 {
			panic("programmer error")
		}
		if len(loc)/2 < group {
			panic("programmer error")
		}
		return str[loc[2*group]:loc[2*group+1]], str[loc[1]:], true
	}
	return "", str, false
}

func download(url, dist string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not download %s: err %d", url, resp.StatusCode)
	}
	f, err := os.Create(dist)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func isKnownFlag(flag string) bool {
	for _, cisFlag := range knownFlags {
		if "--"+flag == cisFlag {
			return true
		}
	}
	return false
}
