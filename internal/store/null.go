package store

import (
	"net/netip"
	"time"
)

// The null* helpers map Go zero values to SQL NULL so empty fields don't
// overwrite real data or violate column types.

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}

func nullInt64(i int64) any {
	if i == 0 {
		return nil
	}
	return i
}

func nullIP(a netip.Addr) any {
	if !a.IsValid() {
		return nil
	}
	return a.String()
}

func ipStr(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}

// nullDate parses common install-date formats into a DATE value, returning NULL
// when the input is empty or unparseable.
func nullDate(s string) any {
	if s == "" {
		return nil
	}
	for _, layout := range []string{"2006-01-02", "20060102", "01/02/2006", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return nil
}
