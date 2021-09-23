package time

import gotime "time"

func Now() Time {
	if faker != nil {
		return faker.Now()
	} else {
		return gotime.Now()
	}
}

func Since(t Time) Duration {
	return Now().Sub(t)
}

func Until(t Time) Duration {
	return t.Sub(Now())
}
