package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
)

type LoadBalancer struct {
	proxies         []Server
	roundRobinCount int
	sync.Mutex
}

type Server struct {
	proxy *httputil.ReverseProxy
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	next := lb.nextIndex()
	lb.proxies[next].proxy.ServeHTTP(w, r)
}

func (lb *LoadBalancer) nextIndex() int {
	proxLen := len(lb.proxies)

	lb.Lock()
	defer lb.Unlock()

	proxInd := lb.roundRobinCount % proxLen
	lb.roundRobinCount++
	return proxInd
}

func main() {
	servers := []Server{
		{
			proxy: &httputil.ReverseProxy{
				Director: func(r *http.Request) {
					r.URL.Scheme = "http"
					r.URL.Host = os.Getenv("FIRST_SERVER_HOST") + ":" + os.Getenv("FIRST_SERVER_PORT")
				},
			},
		},
		{
			proxy: &httputil.ReverseProxy{
				Director: func(r *http.Request) {
					r.URL.Scheme = "http"
					r.URL.Host = os.Getenv("SECOND_SERVER_HOST") + ":" + os.Getenv("SECOND_SERVER_PORT")
				},
			},
		},
	}
	lb := &LoadBalancer{
		proxies:         servers,
		roundRobinCount: 0,
	}

	fmt.Println("Starting GO LOAD BALANCER service on port: " + os.Getenv("PORT"))

	http.ListenAndServe(":"+os.Getenv("PORT"), lb)
}
