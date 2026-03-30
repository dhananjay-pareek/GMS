package leadsdb

import (
	"strconv"
	"time"
)

type UnixTime struct {
	time.Time
}

func (t UnixTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(t.Unix(), 10)), nil
}

func (t *UnixTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || len(data) == 0 {
		return nil
	}

	unix, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}

	t.Time = time.Unix(unix, 0)
	return nil
}
