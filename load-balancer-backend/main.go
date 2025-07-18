package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"sync/atomic"
)

type LoadBalancer struct {
	proxies         []Server
	roundRobinCount uint64
}

type Server struct {
	proxy *httputil.ReverseProxy
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	next := lb.nextIndex()
	lb.proxies[next].proxy.ServeHTTP(w, r)
}

func (lb *LoadBalancer) nextIndex() int {
	return int(atomic.AddUint64(&lb.roundRobinCount, 1) % uint64(len(lb.proxies)))
}

func newReverseProxy(target string) *httputil.ReverseProxy {
	proxy := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = "http"
			r.URL.Host = target
		},
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
		},
	}
	return proxy
}

func main() {
	servers := []Server{
		{proxy: newReverseProxy(os.Getenv("FIRST_SERVER_HOST") + ":" + os.Getenv("FIRST_SERVER_PORT"))},
		{proxy: newReverseProxy(os.Getenv("SECOND_SERVER_HOST") + ":" + os.Getenv("SECOND_SERVER_PORT"))},
	}
	lb := &LoadBalancer{
		proxies:         servers,
		roundRobinCount: 0,
	}

	fmt.Println("Starting GO LOAD BALANCER service on port: " + os.Getenv("PORT"))

	http.ListenAndServe(":"+os.Getenv("PORT"), lb)
}
