package main

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
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


