package handlers

import (
	"fmt"
	"net/http"
)

func NotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, errorPage("404", "Сторінку не знайдено", "Запитана сторінка не існує або була переміщена."))
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusMethodNotAllowed)
	fmt.Fprint(w, errorPage("405", "Метод не дозволено", "Для цієї сторінки використовується неправильний HTTP-метод."))
}

func errorPage(code, title, desc string) string {
	return `<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>RestaurantOS — ` + title + `</title>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet">
<style>
*{font-family:'IBM Plex Sans',sans-serif;margin:0;padding:0;box-sizing:border-box}
body{background:#f8fafc;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#fff;border:1px solid #e2e8f0;border-radius:1rem;padding:2.5rem;text-align:center;max-width:400px;width:100%;box-shadow:0 4px 16px rgba(0,0,0,.06)}
.code{font-size:4rem;font-weight:700;color:#e2e8f0;line-height:1;margin-bottom:.5rem}
.title{font-size:1.1rem;font-weight:600;color:#0f172a;margin-bottom:.5rem}
.desc{font-size:.875rem;color:#64748b;margin-bottom:2rem}
a{display:inline-block;background:#2563eb;color:#fff;text-decoration:none;padding:.5rem 1.25rem;border-radius:.5rem;font-size:.875rem;font-weight:600}
a:hover{background:#3b82f6}
</style>
</head>
<body>
<div class="card">
  <div class="code">` + code + `</div>
  <div class="title">` + title + `</div>
  <div class="desc">` + desc + `</div>
  <a href="/">На головну</a>
</div>
</body>
</html>`
}
