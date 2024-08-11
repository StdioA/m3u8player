package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type ProxyHandler struct {
	redirectBase string
}

var (
	listen       = flag.String("listen", "localhost:6789", "HTTP Server listen address")
	m3u8URL      = flag.String("m3u8", "", "M3U8 URL")
	debug        = flag.Bool("debug", false, "Enable debug mode")
	proxyHandler = new(ProxyHandler)
)

//go:embed static
var staticFolder embed.FS

func (handler *ProxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var (
		proxyURL string
		code     int
		err      error
	)
	if req.URL.Path == "/index.m3u8" {
		proxyURL = *m3u8URL
	} else {
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
		if handler.redirectBase != "" {
			urlBase = handler.redirectBase
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
	defer res.Body.Close()
	code = res.StatusCode
	// 缓存 baseURL，避免后续的重复重定向
	if res.Header.Get("Content-Type") == "application/vnd.apple.mpegurl" {
		handler.redirectBase = res.Request.URL.String()
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

func serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		r.URL.Path = "/index.html"
	}
	filePath := path.Join("static", r.URL.Path) // strings.TrimPrefix(r.URL.Path, "/")
	// Read from static FS
	content, err := staticFolder.ReadFile(filePath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, err.Error())
		return
	}
	w.Write(content)
}

func main() {
	flag.Parse()
	http.HandleFunc("/{$}", serveStatic)
	http.HandleFunc("/index.html", serveStatic)
	http.HandleFunc("/hls.min.js", serveStatic)
	http.HandleFunc("/", proxyHandler.ServeHTTP)

	fmt.Println("Server is listening at", *listen)
	http.ListenAndServe(*listen, nil)
}
