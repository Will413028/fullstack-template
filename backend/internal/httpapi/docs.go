package httpapi

import (
	"net/http"

	"github.com/Will413028/fullstack-template/backend/api"
)

// docsHTML is Swagger UI for the embedded contract. swagger-ui-dist is loaded from jsDelivr pinned to one version
// with Subresource Integrity, so a changed file on the CDN is refused by the browser. The hashes are the sha384 of
// the files at that URL: openssl dgst -sha384 -binary <file> | openssl base64 -A.
const docsHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>API docs</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.0/swagger-ui.css"
  integrity="sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW" crossorigin="anonymous">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.0/swagger-ui-bundle.js"
  integrity="sha384-YDALVcy8kj8yltLBVi1vBiBAUqdxvus673gM8XKwiy6aDUJFXivF/KCufekjYbVf" crossorigin="anonymous"></script>
<script>window.ui = SwaggerUIBundle({url: "/api/openapi.yaml", dom_id: "#swagger-ui"});</script>
</body>
</html>
`

// withDocs serves the contract and Swagger UI in front of next, outside request validation: these routes are not
// part of the contract. Only dev mode mounts it; in prod the paths reach the validator and are 404.
func withDocs(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(api.Spec)
	})
	mux.HandleFunc("GET /api/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsHTML))
	})
	mux.Handle("/", next)
	return mux
}
