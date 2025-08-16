package handler

import (
	"encoding/json"
	"log"

	"github.com/valyala/fasthttp"
)

type HealthInfo struct {
	DefaultFailing          bool `json:"defaultFailing"`
	DefaultMinResponseTime  int  `json:"defaultMinResponseTime"`
	FallbackFailing         bool `json:"fallbackFailing"`
	FallbackMinResponseTime int  `json:"fallbackMinResponseTime"`
}

func HealthUpdateHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var healthInfo HealthInfo
		if err := json.Unmarshal(ctx.PostBody(), &healthInfo); err != nil {
			log.Printf("Failed to unmarshal health info: %v", err)
			ctx.Error("Invalid request body", fasthttp.StatusBadRequest)
			return
		}

		healthMu.Lock()
		defer healthMu.Unlock()

		DefaultProcessorHealth.Failing = healthInfo.DefaultFailing
		DefaultProcessorHealth.MinResponseTime = healthInfo.DefaultMinResponseTime

		FallbackProcessorHealth.Failing = healthInfo.FallbackFailing
		FallbackProcessorHealth.MinResponseTime = healthInfo.FallbackMinResponseTime

		ctx.SetStatusCode(fasthttp.StatusOK)
	}
}
