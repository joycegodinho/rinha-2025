package handler

import (
	"database/db"
	"encoding/json"

	"github.com/valyala/fasthttp"
)

func PurgePaymentsHandler(memoryDB *db.PaymentDB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {

		memoryDB.Clean()

		ctx.SetStatusCode(fasthttp.StatusOK)
		json.NewEncoder(ctx).Encode(map[string]string{
			"message": "All payment records purged successfully",
			"status":  "database_reset",
		})
	}
}