// Package modes provides an adapter that bridges to the original source code
package modes

type Mode string

const (
	ModePull Mode = "pull"
)

func ToStrings(m []Mode) []string {
	var res []string
	for _, mode := range m {
		res = append(res, string(mode))
	}
	return res
}

func (m Mode) MetricTag() string {
	return string(m)
}
