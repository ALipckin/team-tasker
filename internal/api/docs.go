package api

import (
	"net/http"

	"backend/internal/api/docs"
)

// Спецификация генерируется из аннотаций хендлеров: make swagger.
//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g server.go -d .,../domain -o docs --outputTypes go,yaml

const swaggerUIPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Task Service API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>
  window.ui = SwaggerUIBundle({ url: '/swagger/doc.json', dom_id: '#swagger-ui' });
</script>
</body>
</html>`

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIPage))
}

func swaggerSpecHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
}
