package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type ProxyHandler struct{}

var (
	proxyHandler = new(ProxyHandler)
	listen       = flag.String("listen", "localhost:6789", "HTTP Server listen address")
	m3u8URL      = flag.String("m3u8", "", "M3U8 URL")
	debug        = flag.Bool("debug", false, "Enable debug mode")
	baseURL      string
)

//go:embed index.html
var indexHTML []byte

func (*ProxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var (
		proxyURL string
		code     int
		err      error
	)
	switch req.URL.String() {
	case "/index.m3u8":
		proxyURL = *m3u8URL
	case "/", "/index.html":
		w.Write(indexHTML)
		return
	default:
		// Build absolute path base on baseURL & m3u8URL
		// It may starts with "/", or not
		// 一个非常离谱的判断方式：
		// m3u8 ts 如果是绝对 URL，那基本上是不会只有一段，一般都会有很多段
		// TODO: Modify m3u8 content to distinguish between absolute and relative path
		refURL := req.URL
		if strings.Count(refURL.Path, "/") == 1 {
			refURL.Path = strings.TrimLeft(refURL.Path, "/")
		}
		var urlBase = *m3u8URL
		if baseURL == "" {
			urlBase = baseURL
		}
		urlObj, _ := url.Parse(urlBase)
		urlObj = urlObj.ResolveReference(refURL)
		proxyURL = urlObj.String()
	}

	defer func() {
		if *debug {
			log.Println(proxyURL, code)
		}
		if err != nil {
			log.Println(err)
		}
	}()
	// Copy request
	proxyReq, err := http.NewRequest(req.Method, proxyURL, req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, err.Error())
		return
	}
	for k, headers := range req.Header {
		for _, header := range headers {
			proxyReq.Header.Add(k, header)
		}
	}
	// Proxy
	res, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, err.Error())
		return
	}
	code = res.StatusCode
	// 缓存 baseURL，避免后续的重复重定向
	if baseURL == "" {
		baseURL = res.Request.URL.String()
	}

	// Copy response
	for k, headers := range res.Header {
		for _, header := range headers {
			w.Header().Set(k, header)
		}
	}
	w.WriteHeader(res.StatusCode)
	_, err = io.Copy(w, res.Body)
}

func main() {
	flag.Parse()
	fmt.Println("Server is listening at", *listen)
	http.HandleFunc("/*", proxyHandler.ServeHTTP)
	http.ListenAndServe(*listen, nil)
}
