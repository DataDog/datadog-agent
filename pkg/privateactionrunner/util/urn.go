package util

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseRunnerURN(urn string) (string, int64, string, error) {
	urnParts := strings.Split(urn, ":")
	if len(urnParts) != 7 {
		return "", 0, "", fmt.Errorf("invalid URN format: %s", urn)
	}
	orgId, err := strconv.ParseInt(urnParts[5], 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid orgId in URN: %s", urnParts[5])
	}
	return urnParts[6], orgId, urnParts[6], nil
}

func MakeRunnerURN(region string, orgID int64, runnerID string) string {
	return fmt.Sprintf("urn:dd:apps:on-prem-runner:%s:%d:%s", region, orgID, runnerID)
}
