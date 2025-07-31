package handler

import (
	"database/db"
	"encoding/json"

	"github.com/valyala/fasthttp"
)

func PurgePaymentsHandler(memoryDB *db.PaymentDB, fileDB *db.FileDB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {

		if err := fileDB.EraseAll(); err != nil {
			ctx.Error("Failed to purge payments:"+err.Error(), fasthttp.StatusInternalServerError)
			return
		}
		memoryDB.Clean()

		ctx.SetStatusCode(fasthttp.StatusOK)
		json.NewEncoder(ctx).Encode(map[string]string{
			"message": "All payment records purged successfully",
			"status":  "database_reset",
		})
	}
}
