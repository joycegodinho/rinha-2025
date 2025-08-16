package main

import (
	"log"
	"strings"

	"github.com/valyala/fasthttp"
)

func (lb *LoadBalancer) Handler(ctx *fasthttp.RequestCtx) {
	if ctx.IsPost() && strings.HasPrefix(string(ctx.Path()), "/payments") {
		ctx.SetStatusCode(fasthttp.StatusAccepted)

		reqCopy := fasthttp.AcquireRequest()
		respCopy := fasthttp.AcquireResponse()

		ctx.Request.CopyTo(reqCopy)

		go func() {
			defer fasthttp.ReleaseRequest(reqCopy)
			defer fasthttp.ReleaseResponse(respCopy)

			next := lb.nextIndex()
			client := lb.clients[next]

			reqCopy.SetHost("")

			if err := client.Do(reqCopy, respCopy); err != nil {
				log.Printf("Request to backend failed: %v", err)
			}
		}()
		return
	}

	next := lb.nextIndex()
	client := lb.clients[next]

	req := &ctx.Request
	resp := &ctx.Response

	req.SetHost("")

	if err := client.Do(req, resp); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusBadGateway)
	}
}
