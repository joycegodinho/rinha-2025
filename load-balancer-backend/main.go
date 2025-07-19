package main

import (
	"fmt"
	"os"
	"sync/atomic"

	"github.com/valyala/fasthttp"
)

type LoadBalancer struct {
	servers         []string
	roundRobinCount uint64
	client          *fasthttp.Client
}

func (lb *LoadBalancer) Handler(ctx *fasthttp.RequestCtx) {
	next := lb.nextIndex()

	req := &ctx.Request
	resp := &ctx.Response

	req.SetHost(lb.servers[next])

	if err := lb.client.Do(req, resp); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusBadGateway)
	}
}

func (lb *LoadBalancer) nextIndex() int {
	return int(atomic.AddUint64(&lb.roundRobinCount, 1) % uint64(len(lb.servers)))
}

func main() {
	servers := []string{
		os.Getenv("FIRST_SERVER_HOST") + ":" + os.Getenv("FIRST_SERVER_PORT"),
		os.Getenv("SECOND_SERVER_HOST") + ":" + os.Getenv("SECOND_SERVER_PORT"),
	}
	lb := &LoadBalancer{
		servers:         servers,
		roundRobinCount: 0,
		client:          &fasthttp.Client{},
	}

	port := os.Getenv("PORT")
	fmt.Println("Starting GO LOAD BALANCER service on port: " + port)

	fasthttp.ListenAndServe(":"+port, lb.Handler)
}
