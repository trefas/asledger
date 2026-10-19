// Package httpui содержит HTTP-обработчики, общие для команд asledgerd и web.
//
// На этапе каркаса здесь находятся только проверка работоспособности API и
// временная страница web-интерфейса с лого проекта. Полноценные интерфейсы
// появятся в вехах S3 (REST API) и S4 (web).
package httpui

import (
	"encoding/json"
	"html/template"
	"net/http"
)

// pageTemplate — временная страница web-интерфейса с бренд-логотипом.
var pageTemplate = template.Must(template.New("placeholder").Parse(`<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>asledger — разработка</title>
<style>
body { font-family: system-ui, sans-serif; margin: 0; display: flex; min-height: 100vh; align-items: center; justify-content: center; background: #f7f8f5; color: #145214; }
main { text-align: center; padding: 2rem; }
img { width: 160px; }
h1 { margin: 1rem 0 .25rem; }
p { margin: .25rem 0; color: #333; }
</style>
</head>
<body>
<main>
<img src="/assets/asledger-logo.svg" alt="asledger">
<h1>asledger</h1>
<p>Программный комплекс для ведения домашней бухгалтерии</p>
<p>Идёт разработка. Версия: {{.Version}}</p>
</main>
</body>
</html>
`))

// NewWebMux собирает маршруты web-интерфейса: временную страницу и раздачу
// бренд-ассетов из каталога assetsDir по пути /assets/.
func NewWebMux(assetsDir, version string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pageTemplate.Execute(w, struct{ Version string }{version}); err != nil {
			http.Error(w, "не удалось отрисовать страницу", http.StatusInternalServerError)
		}
	})
	return mux
}

// NewAPIMux собирает маршруты REST API. На этапе каркаса доступна только
// проверка работоспособности; остальные маршруты появятся в вехе S3.
func NewAPIMux(version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version,
		})
	})
	return mux
}

// writeJSON отправляет значение в формате JSON с указанным кодом ответа.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
