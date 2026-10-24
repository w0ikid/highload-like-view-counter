//go:build swagger

package bootstrap

import (
	"highload-counters/docs"
	"net/http"
)

func registerSwagger(mux *http.ServeMux) {
	mux.HandleFunc("GET /swagger/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(docs.OpenAPI)
	})
	mux.HandleFunc("GET /swagger/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerHTML))
	})
	mux.HandleFunc("GET /scalar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(scalarHTML))
	})
}

const swaggerHTML = `<!doctype html><html><head><title>Highload counters</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="swagger-ui"></div><script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url:'/swagger/openapi.json',dom_id:'#swagger-ui'});</script></body></html>`

const scalarHTML = `<!doctype html><html><head><title>Highload counters</title></head>
<body><div id="app"></div><script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
<script>Scalar.createApiReference('#app',{url:'/swagger/openapi.json',theme:'deepSpace'});</script></body></html>`
