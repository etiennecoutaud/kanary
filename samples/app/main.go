package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")

func main() {

	httpReqs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_hit",
			Help: "How many HTTP hit, partitioned by pod name.",
		},
		[]string{"hostname"},
	)
	prometheus.MustRegister(httpReqs)
	m := httpReqs.WithLabelValues(os.Getenv("HOSTNAME"))

	flag.Parse()
	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/hit", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("hit by %s", r.Host)
		m.Inc()
	})

	log.Fatal(http.ListenAndServe(*addr, nil))
}
