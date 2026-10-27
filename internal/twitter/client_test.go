package twitter

import (
	"testing"
	"time"
)

func TestIsWeekday(t *testing.T) {
	loc := time.UTC
	// 2026-10-26 is a Monday
	mon := time.Date(2026, 10, 26, 12, 0, 0, 0, loc)
	sat := time.Date(2026, 10, 24, 12, 0, 0, 0, loc)
	sun := time.Date(2026, 10, 25, 12, 0, 0, 0, loc)
	if !IsWeekday(mon, loc) {
		t.Fatalf("expected Monday weekday")
	}
	if IsWeekday(sat, loc) || IsWeekday(sun, loc) {
		t.Fatalf("expected weekend not weekday")
	}
}
