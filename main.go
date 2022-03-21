package main

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

type Backend struct {
	URL *url.URL
	Alive bool
	mux sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

type ServerPool struct {
	backens []*Backend
	currIndex uint64
}