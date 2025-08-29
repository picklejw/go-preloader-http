package HttpPreloader

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func isDev() bool {
	return os.Getenv("IS_DEV") == "true"
}

type PreloadRouteMap struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

type InterceptWriter struct {
	Headers    http.Header `json:"headers"`
	StatusCode int         `json:"statusCode"`
	Body       string      `json:"body"`
	buf        *bytes.Buffer
}

func NewInterceptWriter() *InterceptWriter {
	return &InterceptWriter{
		Headers:    make(http.Header),
		StatusCode: http.StatusOK,
		buf:        new(bytes.Buffer),
	}
}

func (iw *InterceptWriter) Header() http.Header {
	return iw.Headers
}

func (iw *InterceptWriter) WriteHeader(code int) {
	iw.StatusCode = code
}

func (iw *InterceptWriter) Write(b []byte) (int, error) {
	iw.buf.Write(b)           // capture body
	iw.Body = iw.buf.String() // keep as string
	return len(b), nil
}

type HttpPreloaderContext struct {
	routes map[string]map[string]PreloadRouteMap // method -> path -> route
}

func (ctx *HttpPreloaderContext) Get(pattern string, handler http.HandlerFunc) {
	ctx.addRoute(http.MethodGet, pattern, handler)
}
func (ctx *HttpPreloaderContext) Post(pattern string, handler http.HandlerFunc) {
	ctx.addRoute(http.MethodPost, pattern, handler)
}
func (ctx *HttpPreloaderContext) Put(pattern string, handler http.HandlerFunc) {
	ctx.addRoute(http.MethodPut, pattern, handler)
}
func (ctx *HttpPreloaderContext) Patch(pattern string, handler http.HandlerFunc) {
	ctx.addRoute(http.MethodPatch, pattern, handler)
}
func (ctx *HttpPreloaderContext) Delete(pattern string, handler http.HandlerFunc) {
	ctx.addRoute(http.MethodDelete, pattern, handler)
}
func (ctx *HttpPreloaderContext) Handle(method, pattern string, handler http.HandlerFunc) {
	ctx.addRoute(method, pattern, handler)
}

func (ctx *HttpPreloaderContext) addRoute(method, pattern string, handler http.HandlerFunc) {
	if ctx.routes[method] == nil {
		ctx.routes[method] = make(map[string]PreloadRouteMap)
	}
	ctx.routes[method][pattern] = PreloadRouteMap{Path: pattern, Handler: handler}
}

func NewHttpPreloaderContext() *HttpPreloaderContext {
	return &HttpPreloaderContext{
		routes: make(map[string]map[string]PreloadRouteMap),
	}
}

func requestIsDefaultIndex(path string) bool {
	lowerURI := strings.ToLower(path)
	segments := strings.Split(lowerURI, "/")
	lastSegment := segments[len(segments)-1]

	if lastSegment == "" || lastSegment == "index.html" || lastSegment == "index.htm" {
		return true
	}
	if !strings.Contains(lastSegment, ".") {
		return true
	}
	return false
}

func (ctx *HttpPreloaderContext) HttpPreloader(
	next http.Handler,
	apiPrefix string,
	reactAppBuildRoot string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Path

		if !strings.HasPrefix(uri, apiPrefix) { // this is a request for a page from React, not a /api request
			// Are we in dev mode or static build?
			useProxy := false
			if reactAppBuildRoot == "" {
				useProxy = true
			} else {
				if stat, err := os.Stat(reactAppBuildRoot); err != nil || !stat.IsDir() {
					useProxy = true
				}
			}

			reactIndexText := ""

			if requestIsDefaultIndex(r.URL.Path) {
				if useProxy {
					// fetch index.html from proxy
					target := "http://localhost:3000/index.html"
					resp, err := http.Get(target)
					if err != nil {
						http.Error(w, "failed to fetch index.html from proxy", http.StatusBadGateway)
						return
					}
					defer resp.Body.Close()
					body, _ := io.ReadAll(resp.Body)
					reactIndexText = string(body)
				} else {
					// fetch index.html from disk
					reqPath := filepath.Join(reactAppBuildRoot, "index.html")
					body, err := os.ReadFile(reqPath)
					if err != nil {
						http.Error(w, "failed to read index.html from disk", http.StatusInternalServerError)
						return
					}
					reactIndexText = string(body)
				}
			} else { // some other resource, like a file
				if useProxy {
					target := "http://localhost:3000"
					url, _ := url.Parse(target)
					proxy := httputil.NewSingleHostReverseProxy(url)
					proxy.ServeHTTP(w, r)
				} else {
					reqPath := filepath.Join(reactAppBuildRoot, r.URL.Path)
					if stat, err := os.Stat(reqPath); err == nil && !stat.IsDir() {
						http.ServeFile(w, r, reqPath)
					} else {
						http.NotFound(w, r)
					}
				}
				return
			}

			// we need to break down the path, so this code is called for "/" and "/item?id=shit"
			// this once this is built then

			// Build the preloader handler requests:
			parts := strings.SplitN(r.RequestURI, "?", 2)
			basePath := parts[0]
			pathSegments := strings.Split(basePath, "/")

			preloadRequests := map[string]*InterceptWriter{}
			currentPathReq := ""

			for i, segment := range pathSegments {
				if len(currentPathReq) > 0 && currentPathReq[len(currentPathReq)-1] == '/' {
					currentPathReq = currentPathReq + segment
				} else {
					currentPathReq = currentPathReq + "/" + segment
				}
				if route, ok := ctx.routes[r.Method][currentPathReq]; ok {
					preloadWriter := NewInterceptWriter()
					if i == len(pathSegments)-1 {
						route.Handler(preloadWriter, r)
					} else {
						// Hacking to remove query parameters when not full path request handled:
						rCopy := r.Clone(r.Context())
						urlCopy := *rCopy.URL
						urlCopy.RawQuery = ""
						rCopy.URL = &urlCopy
						route.Handler(preloadWriter, rCopy)
					}

					if i == len(pathSegments)-1 {
						currentPathReq = r.RequestURI // no need to get fancy, its just the base request
					}

					preloadRequests[currentPathReq] = preloadWriter
				}
			}

			// Done handoing all data requests, now bundle it in .html request:

			responseJSON, _ := json.Marshal(preloadRequests)
			varString := "<script>window.httpPreload=" + string(responseJSON) + "</script>"

			if idx := strings.LastIndex(strings.ToLower(reactIndexText), "</body>"); idx != -1 {
				reactIndexText = reactIndexText[:idx] + varString + reactIndexText[idx:]
			} else {
				reactIndexText += varString
			}

			if iw, ok := preloadRequests[r.RequestURI]; ok && iw != nil {
				if iw.StatusCode != 404 {
					for k, vv := range iw.Headers {
						for _, v := range vv {
							w.Header().Add(k, v)
						}
					}
				}
			}

			w.Write([]byte(reactIndexText))
		} else {
			// this is a API request, need to handler by prepending`apiPrefix`
			println(r.URL.Path)
			directApiPath := strings.TrimPrefix(r.URL.Path, apiPrefix)
			if len(directApiPath) == 0 {
				directApiPath = "/" // default to root
			}

			// Check if route exists
			if route, ok := ctx.routes[r.Method][directApiPath]; ok {
				preloadWriter := NewInterceptWriter()
				route.Handler(preloadWriter, r)
				w.Write(preloadWriter.buf.Bytes())
			}
		}
	})
}
