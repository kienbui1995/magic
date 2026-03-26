package gateway

import (
	_ "embed"
	"net/http"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}
