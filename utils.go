package main

import (
	"strings"
	"time"
)

var kyivLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		panic("failed to load Europe/Kyiv timezone: " + err.Error())
	}
	return loc
}()

func nowKyiv() time.Time {
	return time.Now().In(kyivLocation)
}

type datetime struct {
	time.Time
}

func (dt datetime) MarshalJSON() ([]byte, error) {
	if dt.IsZero() {
		return nil, nil
	}
	return []byte(`"` + dt.Format("15:04 02.01.2006") + `"`), nil
}

func (dt *datetime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	parsed, err := time.ParseInLocation("15:04 02.01.2006", s, kyivLocation)
	if err != nil {
		return err
	}
	dt.Time = parsed
	return nil
}

func (dt *datetime) Equal(other *datetime) bool {
	if dt == other {
		return true
	}
	if dt == nil || other == nil {
		return false
	}
	return dt.Time.Equal(other.Time)
}
