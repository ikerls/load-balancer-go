package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MAX_RETRIES = 3
)

type Backend struct {
	URL *url.URL
	Alive bool
	mux sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}
 // using mutex to make sure only one goroutine (thread) is updating the index
func (server * Backend) IsAlive () (alive bool) {
	server.mux.RLock()
	alive = server.Alive
	server.mux.RUnlock()
	return
}

func (server * Backend) SetAlive (alive bool) {
	server.mux.Lock()
	server.Alive = alive
	server.mux.Unlock()
}

type ServerPool struct {
	backens []*Backend
	currIndex uint64
}

func (servers *ServerPool) NextIndex() int {
	return int(atomic.AddUint64(&servers.currIndex, 1) % uint64(len(servers.backens)))
}

// returns the next available backend
func (servers *ServerPool) GetNextBackend() *Backend {
	next := servers.NextIndex()
	serversLenth := len(servers.backens) + next
	for i := next; i < serversLenth; i++ {
		index := i % len(servers.backens)

		if servers.backens[index].IsAlive() {
			// store the current index if its not the same server
			if i != next {
				atomic.StoreUint64(&servers.currIndex, uint64(index))
			}
		}
		return servers.backens[index]
		}
		return nil
}

func loadBalancer (writer http.ResponseWriter, request *http.Request) {
	server := servers.GetNextBackend()
	if server != nil {
		server.ReverseProxy.ServeHTTP(writer, request)
		return
	}
	 http.Error(writer, "Service Unavailable", http.StatusServiceUnavailable)
}

var servers ServerPool

func GetRetryFromContext (request http.Request) int {
	if retries, ok := request.Context().Value(retriesKey).(int); ok {
		return retries
	}
	return 0
}

func (s *ServerPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	for _, server := range s.backens {
		if server.URL.String() == backendUrl.String() {
			server.SetAlive(alive)
			return
		}
	}
}

func GetAttemptsFromContext(r *http.Request) int {
	if attempts, ok := r.Context().Value(attemptsKey).(int); ok {
		return attempts
	}
	return 0
}

func main() {

	var port int

	server := http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(loadBalancer),
	}

	proxy := httputil.NewSingleHostReverseProxy(serverUrl)
	proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
		log.Println("Proxy Error: ", e.Error())
		retries := GetRetryFromContext(request)
		if retries < MAX_RETRIES {
			if <-time.After(10 * time.Millisecond) {
				ctx := context.WithValue(request.Context(), retriesKey, retries + 1)
				proxy.ServeHTTP(writer, request.WithContext(ctx))
			}
			return
		}
		 // if we have retried 3 times, mark the server as down
		servers.MarkBackendStatus(serverUrl, false)
		attemps := GetAttemptsFromContext(request)
		log.Printf("%s %s retry: %d \n", request.Method, request.URL.String(), attemps)
		ctx := context.WithValue(request.Context(), attemptsKey, attemps + 1)
		loadBalancer(writer, request.WithContext(ctx))

}


