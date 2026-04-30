package dtek

import (
	"log/slog"
	"strings"
	"time"
)

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
	if dt == nil || other == nil {
		return dt == other
	}
	return dt.Time.Equal(other.Time)
}

var kyivLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		slog.Error("failed to load timezone", "tz", "Europe/Kyiv", "error", err)
		panic(err)
	}
	return loc
}()
