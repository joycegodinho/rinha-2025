package health

import (
	"encoding/json"
	// "fmt"
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

type HealthManager struct {
	Processor string
	Endpoint  string
}

func (h *HealthManager) CheckAndUpdateHealth() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		h.updateHealth()
	}
}

func (h *HealthManager) updateHealth() {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(h.Endpoint)
	req.Header.SetMethod(fasthttp.MethodGet)

	client := &fasthttp.Client{
		MaxConnsPerHost:               10,
		ReadTimeout:                   2 * time.Second,
		WriteTimeout:                  2 * time.Second,
		MaxIdleConnDuration:           10 * time.Second,
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}

	err := client.Do(req, resp)
	if err != nil {
		log.Printf("Health check for %s failed with error: %v", h.Processor, err)
		h.SaveHealth(true, 9999)
		return
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		log.Printf("Health check for %s received status code: %d", h.Processor, resp.StatusCode())
		h.SaveHealth(true, 9999)
		return
	}

	body := resp.Body()

	var res struct {
		Failing         bool `json:"failing"`
		MinResponseTime int  `json:"minResponseTime"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		log.Printf("Failed to unmarshal health check response for %s: %v", h.Processor, err)
		h.SaveHealth(true, 9999)
		return
	}

	h.SaveHealth(res.Failing, res.MinResponseTime)
}

func (h *HealthManager) SaveHealth(failing bool, minResp int) {
	mu.Lock()
	defer mu.Unlock()

	health := ProcessorHealth{
		Failing:         failing,
		MinResponseTime: minResp,
	}

	if h.Processor == "default" {
		// fmt.Printf("Updated health for %s: %+v\n", h.Processor, health)
		DefaultHealth = health
	} else {
		// fmt.Printf("Updated health for %s: %+v\n", h.Processor, health)
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
