package route

import (
	"net/http"
	//"github.com/prometheus/client_golang/prometheus"
)

// Healthz simple healthcheck endpoint
// HTTP/200
func Healthz(w http.ResponseWriter, r *http.Request) {
}
