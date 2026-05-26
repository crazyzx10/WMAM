package storage

import (
	"strings"
	"time"
)

const displayTimeLayout = "2006-01-02 15:04:05"

var displayTimeLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return location
}()

func formatDisplayTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	zonedLayouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, layout := range zonedLayouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.In(displayTimeLocation).Format(displayTimeLayout)
		}
	}

	utcLayouts := []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	}
	for _, layout := range utcLayouts {
		parsed, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return parsed.In(displayTimeLocation).Format(displayTimeLayout)
		}
	}

	return value
}
