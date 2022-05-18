package tag

import (
	"fmt"
	"os"
	"strings"
)

func GetBaseTags() []string {
	if len(os.Getenv("K_SERVICE")) > 0 && len(os.Getenv("K_REVISION")) > 0 {
		return []string{
			fmt.Sprintf("revision:%s", strings.ToLower(os.Getenv("K_REVISION"))),
			fmt.Sprintf("service:%s", strings.ToLower(os.Getenv("K_SERVICE"))),
		}
	}
	return []string{}
}
