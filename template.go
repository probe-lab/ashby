package main

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
)

func ExecuteTemplate(ctx context.Context, source string, cfg *PlotConfig) (string, error) {
	type ExecutionContext struct {
		Now         time.Time
		StartOfDay  time.Time
		StartOfWeek time.Time
	}

	// See http://masterminds.github.io/sprig/
	fm := sprig.FuncMap()
	fm["timestamptz"] = pgTimestampTZ
	fm["timestamp"] = pgTimestamp
	fm["simpledate"] = simpleDateFormat
	fm["isodate"] = isoDateFormat
	fm["dayModify"] = dayModify     // a version of sprig's dateModify that accepts a number of days
	fm["weekModify"] = weekModify   // a version of sprig's dateModify that accepts a number of weeks
	fm["monthModify"] = monthModify // a version of sprig's dateModify that accepts a number of months
	fm["toUpper"] = strings.ToUpper
	fm["toTitle"] = strings.ToTitle

	t, err := template.New("").Funcs(fm).Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse query template: %w", err)
	}

	data := map[string]any{
		"Now":         cfg.BasisTime,
		"StartOfHour": cfg.BasisTime.Truncate(time.Hour),
		"StartOfDay":  cfg.BasisTime.Truncate(24 * time.Hour),
		"StartOfWeek": cfg.BasisTime.Truncate(7 * 24 * time.Hour),

		// The following are useful when formatting dates that are immediately before the start of the period
		// They are not really suitable for use as the end of a range in a query.
		"EndOfPreviousHour":   cfg.BasisTime.Truncate(time.Hour).Add(-time.Nanosecond),
		"EndOfPreviousDay":    cfg.BasisTime.Truncate(24 * time.Hour).Add(-time.Nanosecond),
		"EndOfPreviousWeek":   cfg.BasisTime.Truncate(7 * 24 * time.Hour).Add(-time.Nanosecond),
		"StartOfPreviousWeek": cfg.BasisTime.Truncate(7 * 24 * time.Hour).Add(-7 * 24 * time.Hour),
		"Params":              cfg.TemplateParams,
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, data); err != nil {
		return "", fmt.Errorf("execute query template: %w", err)
	}

	return buf.String(), nil
}

func pgTimestampTZ(t time.Time) string {
	return "'" + t.Format("2006-01-02 15:04:05 Z") + "'::timestamptz"
}

func pgTimestamp(t time.Time) string {
	return "'" + t.Format("2006-01-02 15:04:05") + "'::timestamp"
}

func simpleDateFormat(t time.Time) string {
	return t.Format("2 Jan 2006")
}

func isoDateFormat(t time.Time) string {
	return t.Format(time.RFC3339)
}

func dayModify(fmt string, date time.Time) time.Time {
	n, err := strconv.Atoi(fmt)
	if err != nil {
		return date
	}

	return date.Add(time.Duration(n) * time.Hour * 24)
}

func weekModify(fmt string, date time.Time) time.Time {
	n, err := strconv.Atoi(fmt)
	if err != nil {
		return date
	}

	return date.Add(time.Duration(n) * time.Hour * 24 * 7)
}

func monthModify(fmt string, date time.Time) time.Time {
	n, err := strconv.Atoi(fmt)
	if err != nil {
		return date
	}

	return date.AddDate(0, n, 0)
}
