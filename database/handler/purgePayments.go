package handler

import (
	"database/db"
	"encoding/json"

	// "log"
	"net/http"
)

func PurgePaymentsHandler(memoryDB *db.PaymentDB, fileDB *db.FileDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if err := fileDB.EraseAll(); err != nil {
			http.Error(w, "Failed to purge payments:"+err.Error(), http.StatusInternalServerError)
			return
		}
		memoryDB.Clean()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "All payment records purged successfully",
			"status":  "database_reset",
		})
	}
}
