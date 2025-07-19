package health

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/valyala/fasthttp"
)

type ProcessorHealth struct {
	Failing         bool      `json:"failing"`
	MinResponseTime int       `json:"minResponseTime"`
	LastChecked     time.Time `json:"lastChecked,omitempty"`
}

type HealthManager struct {
	Redis     *redis.Client
	Processor string
	Endpoint  string
	Token     string
	LockKey   string
	Ctx       context.Context
}

func (h *HealthManager) CheckAndUpdateHealth() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		h.updateHealth()
	}
}

func (h *HealthManager) updateHealth() {
	ok, err := h.Redis.SetNX(h.Ctx, "lock:health:"+h.Processor, "1", 2*time.Second).Result()
	if err != nil || !ok {
		return
	}

	statusCode, body, err := fasthttp.GetTimeout(nil, h.Endpoint, 2*time.Second)
	if err != nil || statusCode != fasthttp.StatusOK {
		h.SaveHealthToRedis(true, 9999)
		return
	}

	var res struct {
		Failing         bool `json:"failing"`
		MinResponseTime int  `json:"minResponseTime"`
	}

	if err := json.Unmarshal(body, &res); err != nil {
		h.SaveHealthToRedis(true, 9999)
		return
	}
	h.SaveHealthToRedis(res.Failing, res.MinResponseTime)
}

func (h *HealthManager) SaveHealthToRedis(failing bool, minResp int) {
	health := ProcessorHealth{
		Failing:         failing,
		MinResponseTime: minResp,
		LastChecked:     time.Now().UTC(),
	}
	data, _ := json.Marshal(health)
	h.Redis.Set(h.Ctx, "health:"+h.Processor, data, 6*time.Second)
}

func (h *HealthManager) GetHealth() (*ProcessorHealth, error) {
	data, err := h.Redis.Get(h.Ctx, "health:"+h.Processor).Result()
	if err != nil {
		return nil, err
	}
	var hp ProcessorHealth
	err = json.Unmarshal([]byte(data), &hp)
	return &hp, err
}
