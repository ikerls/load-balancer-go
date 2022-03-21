package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MAX_RETRIES = 3
	RETRIES_KEY
	ATTEMPTS int = iota
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
	backends []*Backend
	currIndex uint64
}

func (servers *ServerPool) NextIndex() int {
	return int(atomic.AddUint64(&servers.currIndex, 1) % uint64(len(servers.backends)))
}

// returns the next available backend
func (servers *ServerPool) GetNextBackend() *Backend {
	next := servers.NextIndex()
	serversLength := len(servers.backends) + next
	for i := next; i < serversLength; i++ {
		index := i % len(servers.backends)

		if servers.backends[index].IsAlive() {
			// store the current index if its not the same server
			if i != next {
				atomic.StoreUint64(&servers.currIndex, uint64(index))
			}
		}
		return servers.backends[index]
		}
		return nil
}

func loadBalancer (writer http.ResponseWriter, request *http.Request) {
	attempts := GetAttemptsFromContext(request)
	if attempts >= MAX_RETRIES {
		log.Printf("%s %s retry: %d \n", request.Method, request.URL.String(), attempts)
		http.Error(writer, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	server := servers.GetNextBackend()
	if server != nil {
		server.ReverseProxy.ServeHTTP(writer, request)
		return
	}
	 http.Error(writer, "Service Unavailable", http.StatusServiceUnavailable)
}

var servers ServerPool

func (s *ServerPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	for _, server := range s.backends {
		if server.URL.String() == backendUrl.String() {
			server.SetAlive(alive)
			return
		}
	}
}

func GetRetryFromContext(request *http.Request) int {
	if retries, ok := request.Context().Value(RETRIES_KEY).(int); ok {
		return retries
	}
	return 0
}

func GetAttemptsFromContext(r *http.Request) int {
	if attempts, ok := r.Context().Value(ATTEMPTS).(int); ok {
		return attempts
	}
	return 0
}

func isBackendAlive (url *url.URL) bool {
	timeOut := time.Second * 2
	// establishing a TCP connection (ping)
	conn, err := net.DialTimeout("tcp", url.Host, timeOut)
	if err != nil {
		log.Println("error connecting: ", err)
		return false
	}
	_ = conn.Close()
	return true
}

func (s *ServerPool) HealthCheck() {
	for _, server := range s.backends {
		status := "up"
		alive := isBackendAlive(server.URL)
		if !alive {
			status = "down"
		}
		log.Printf("%s: %s\n", server.URL.String(), status)
	}
}

// check status of the backends every 2 mins
func HealthCheck() {
	timer := time.NewTicker(time.Second * 20)
	for {
		select {
		case <-timer.C:
			log.Println("Checking Backends")
			servers.HealthCheck()
			log.Println("Checking Backends Done")
		}
	}
}

func (servers *ServerPool) AddBackend(backend *Backend) {
	servers.backends = append(servers.backends, backend)
}

func main() {
	var port int
	var backends string
	flag.StringVar(&backends, "backends", "", "backends to load balanced, use commas to separate")
	flag.IntVar(&port, "port", 8080, "port to listen")
	flag.Parse()

	if len(backends) == 0 {
		log.Fatal("Please specify backends to load balance on")
	}

	tokens := strings.Split(backends, ",")
	for _, token := range tokens {
		serverUrl, err := url.Parse(token)
		if err != nil {
			log.Fatal(err)
		}
		proxy := httputil.NewSingleHostReverseProxy(serverUrl)
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
			log.Println("Proxy Error: ", e.Error())
			retries := GetRetryFromContext(request)
			if retries < MAX_RETRIES {
				select {
				case <-time.After(10 * time.Millisecond):
						ctx := context.WithValue(request.Context(), RETRIES_KEY, retries + 1)
						proxy.ServeHTTP(writer, request.WithContext(ctx))
				}
				return
			}
			// if we have retried 3 times, mark the server as down
			servers.MarkBackendStatus(serverUrl, false)
			attempsFromContext := GetAttemptsFromContext(request)
			log.Printf("%s %s retry: %d \n", request.Method, request.URL.String(), attempsFromContext)
			ctx := context.WithValue(request.Context(), ATTEMPTS, attempsFromContext + 1)
			loadBalancer(writer, request.WithContext(ctx))
		}
		servers.AddBackend(&Backend{
			URL: serverUrl,
			Alive: true,
			ReverseProxy: proxy,
		})
	}

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(loadBalancer),
	}

	// start health checking
	go HealthCheck()

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}


