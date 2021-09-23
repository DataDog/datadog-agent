package time

import gotime "time"

// this file just copies items from the "real" time module

const (
	ANSIC       = gotime.ANSIC
	UnixDate    = gotime.UnixDate
	RubyDate    = gotime.RubyDate
	RFC822      = gotime.RFC822
	RFC822Z     = gotime.RFC822Z
	RFC850      = gotime.RFC850
	RFC1123     = gotime.RFC1123
	RFC1123Z    = gotime.RFC1123Z
	RFC3339     = gotime.RFC3339
	RFC3339Nano = gotime.RFC3339Nano
	Kitchen     = gotime.Kitchen
	Stamp       = gotime.Stamp
	StampMilli  = gotime.StampMilli
	StampMicro  = gotime.StampMicro
	StampNano   = gotime.StampNano
)

const (
	Hour        = gotime.Hour
	Minute      = gotime.Minute
	Second      = gotime.Second
	Millisecond = gotime.Millisecond
	Microsecond = gotime.Microsecond
	Nanosecond  = gotime.Nanosecond
)

type Duration = gotime.Duration

func ParseDuration(s string) (Duration, error) {
	return gotime.ParseDuration(s)
}

type Location = gotime.Location

func FixedZone(name string, offset int) *Location {
	return gotime.FixedZone(name, offset)
}

func LoadLocation(name string) (*Location, error) {
	return gotime.LoadLocation(name)
}

func LoadLocationFromTZData(name string, data []byte) (*Location, error) {
	return LoadLocationFromTZData(name, data)
}

type Month = gotime.Month

type ParseError = gotime.ParseError

type Time = gotime.Time

func Date(year int, month Month, day, hour, min, sec, nsec int, loc *Location) Time {
	return gotime.Date(year, month, day, hour, min, sec, nsec, loc)
}

func Parse(layout, value string) (Time, error) {
	return gotime.Parse(layout, value)
}

func ParseInLocation(layout, value string, loc *Location) (Time, error) {
	return gotime.ParseInLocation(layout, value, loc)
}

func Unix(sec int64, nsec int64) Time {
	return gotime.Unix(sec, nsec)
}
