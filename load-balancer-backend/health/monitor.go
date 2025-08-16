package health

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

var (
	DefaultHealth  ProcessorHealth = ProcessorHealth{Failing: false, MinResponseTime: 0}
	FallbackHealth ProcessorHealth = ProcessorHealth{Failing: false, MinResponseTime: 0}
	mu             sync.RWMutex
)

type ProcessorHealth struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

type HealthInfo struct {
	DefaultFailing          bool `json:"defaultFailing"`
	DefaultMinResponseTime  int  `json:"defaultMinResponseTime"`
	FallbackFailing         bool `json:"fallbackFailing"`
	FallbackMinResponseTime int  `json:"fallbackMinResponseTime"`
}

type HealthManager struct {
	Processor            string
	ProcessorEndpoint    string
	HealthUpdateEndpoint string
	ProcessorClient      *fasthttp.Client
	HealthUpdateClient   []*fasthttp.HostClient
}

func (h *HealthManager) CheckAndUpdateHealth() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		h.updateHealth()
	}
}

func (h *HealthManager) updateHealth() {

	// --- Step 1: Pull Individual Health from Processor (GET request) ---
	reqPull := fasthttp.AcquireRequest()
	respPull := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(reqPull)
	defer fasthttp.ReleaseResponse(respPull)

	reqPull.SetRequestURI(h.ProcessorEndpoint)
	reqPull.Header.SetMethod(fasthttp.MethodGet)

	err := h.ProcessorClient.Do(reqPull, respPull)
	if err != nil {
		log.Printf("Health check for %s failed to pull health: %v", h.Processor, err)
		h.SaveIndividualHealth(true, 9999)
	} else if respPull.StatusCode() != fasthttp.StatusOK {
		log.Printf("Health check for %s pulled non-OK status: %d", h.Processor, respPull.StatusCode())
		h.SaveIndividualHealth(true, 9999)
	} else {
		var res struct {
			Failing         bool `json:"failing"`
			MinResponseTime int  `json:"minResponseTime"`
		}
		if err := json.Unmarshal(respPull.Body(), &res); err != nil {
			log.Printf("Failed to unmarshal pulled health response for %s: %v", h.Processor, err)
			h.SaveIndividualHealth(true, 9999)
		} else {
			h.SaveIndividualHealth(res.Failing, res.MinResponseTime)
		}
	}

	// --- Step 2: Push Aggregated Health to Backend (POST request) ---
	reqPush := fasthttp.AcquireRequest()
	respPush := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(reqPush)
	defer fasthttp.ReleaseResponse(respPush)

	reqPush.SetRequestURI(h.HealthUpdateEndpoint)
	reqPush.Header.SetMethod(fasthttp.MethodPost)
	reqPush.Header.SetContentType("application/json")
	reqPush.SetHost("unix")

	mu.RLock()
	currentDefaultHealth := DefaultHealth
	currentFallbackHealth := FallbackHealth
	mu.RUnlock()

	healthInfo := HealthInfo{
		DefaultFailing:          currentDefaultHealth.Failing,
		DefaultMinResponseTime:  currentDefaultHealth.MinResponseTime,
		FallbackFailing:         currentFallbackHealth.Failing,
		FallbackMinResponseTime: currentFallbackHealth.MinResponseTime,
	}

	healthInfoJSON, err := json.Marshal(healthInfo)
	if err != nil {
		log.Printf("Failed to marshal aggregated health info for %s: %v", h.Processor, err)
		return
	}

	reqPush.SetBody(healthInfoJSON)

	err = h.HealthUpdateClient[0].Do(reqPush, respPush)
	if err != nil {
		log.Printf("Health update push for %s failed with error: %v", h.Processor, err)
		return
	}

	if respPush.StatusCode() != fasthttp.StatusOK {
		log.Printf("Health update push for %s received non-OK status code: %d", h.Processor, respPush.StatusCode())
		return
	}

	err = h.HealthUpdateClient[1].Do(reqPush, respPush)
	if err != nil {
		log.Printf("Health update push for %s failed with error: %v", h.Processor, err)
		return
	}

	if respPush.StatusCode() != fasthttp.StatusOK {
		log.Printf("Health update push for %s received non-OK status code: %d", h.Processor, respPush.StatusCode())
		return
	}
}

func (h *HealthManager) SaveIndividualHealth(failing bool, minResp int) {
	mu.Lock()
	defer mu.Unlock()

	health := ProcessorHealth{
		Failing:         failing,
		MinResponseTime: minResp,
	}

	if h.Processor == "default" {
		DefaultHealth = health
	} else {
		FallbackHealth = health
	}
}

func GetHealth(processor string) ProcessorHealth {
	mu.RLock()
	defer mu.RUnlock()

	if processor == "default" {
		return DefaultHealth
	} else {
		return FallbackHealth
	}
}
