package ec2

import (
	"fmt"
	"os"
	"strings"

	componentsos "github.com/DataDog/test-infra-definitions/components/os"
)

func getWindowsOpenSSHUserData(publicKeyPath string) (string, error) {
	publicKey, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", err
	}

	return buildAWSPowerShellUserData(
			componentsos.WindowsSetupSSHScriptContent,
			windowsPowerShellArgument{name: "authorizedKey", value: string(publicKey)},
		),
		nil
}

type windowsPowerShellArgument struct {
	name  string
	value string
}

func (a windowsPowerShellArgument) String() string {
	return fmt.Sprintf("-%s %s", a.name, a.value)
}

func buildAWSPowerShellUserData(scriptContent string, arguments ...windowsPowerShellArgument) string {
	for _, arg := range arguments {
		scriptContent = strings.ReplaceAll(scriptContent, fmt.Sprintf("$%s", arg.name), fmt.Sprintf("'%s'", arg.value))
	}

	scriptLines := strings.Split(scriptContent, "\n")
	userDataLines := make([]string, 0, len(scriptLines)+6+len(arguments))
	userDataLines = append(userDataLines, "<powershell>")
	for _, line := range scriptLines {
		// indent script lines by one tab
		userDataLines = append(userDataLines, fmt.Sprintf("		%s", line))
	}
	userDataLines = append(userDataLines, "</powershell>")
	userDataLines = append(userDataLines, "<persist>true</persist>")

	return strings.Join(userDataLines, "\n")
}
