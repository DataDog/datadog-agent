package checkconfig

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util"
)

func buildDeviceID(origTags []string) (string, []string) {
	h := fnv.New64()
	var tags []string
	for _, tag := range origTags {

		// When using snmp_listener, the `autodiscovery_subnet` tag is passed
		// via autodiscovery template. We are ignoring it to avoid deviceID
		// being changed when we change subnet e.g. 10.10.0.0/29 -> 10.10.0.0/30
		if strings.HasPrefix(tag, subnetTagPrefix+":") {
			continue
		}

		tags = append(tags, tag)
	}
	tags = util.SortUniqInPlace(tags)
	for _, tag := range tags {
		// the implementation of h.Write never returns a non-nil error
		_, _ = h.Write([]byte(tag))
	}
	return strconv.FormatUint(h.Sum64(), 16), tags
}
