package handler

import (
	"database/db"
	"encoding/json"
	"fmt"
	"time"

	"github.com/valyala/fasthttp"
)

func SummaryHandler(d *db.PaymentDB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {

		if !ctx.IsGet() {
			ctx.Error("Method not allowed", fasthttp.StatusMethodNotAllowed)
			return
		}

		from := string(ctx.QueryArgs().Peek("from"))
		to := string(ctx.QueryArgs().Peek("to"))

		var (
			fromTime time.Time
			toTime   time.Time
			err      error
		)

		if from != "" {
			fromTime, err = parseTime(from)
			if err != nil {
				ctx.Error("Invalid 'from' parameter", fasthttp.StatusBadRequest)
				return
			}
		} else {
			fromTime = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		}

		if to != "" {
			toTime, err = parseTime(to)
			if err != nil {
				ctx.Error("Invalid 'to' parameter", fasthttp.StatusBadRequest)
				return
			}
		} else {
			toTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
		}

		if fromTime.After(toTime) {
			ctx.Error("'from' time cannot be after 'to' time", fasthttp.StatusBadRequest)
			return
		}

		summary := d.QuerySummary(fromTime, toTime)

		// Return response
		ctx.SetContentType("application/json")
		json.NewEncoder(ctx).Encode(summary)
	}
}

func parseTime(s string) (time.Time, error) {
	// Try standard RFC3339 format first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try without timezone but with milliseconds
	if t, err := time.Parse("2006-01-02T15:04:05.999", s); err == nil {
		return t, nil
	}

	// Try without milliseconds
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t, nil
	}

	// Try with just date
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	// Try Unix timestamp
	if t, err := time.Parse(time.UnixDate, s); err == nil {
		return t, nil
	}

	// Return a proper error
	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}
