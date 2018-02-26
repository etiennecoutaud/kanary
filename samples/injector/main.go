package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")

func main() {

	perf := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hit_duration",
			Help: "Current delay foreach hit",
		},
		[]string{"target"},
	)

	prometheus.MustRegister(perf)

	flag.Parse()
	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		port := r.URL.Query().Get("port")
		counter, _ := strconv.Atoi(r.URL.Query().Get("counter"))
		targetUrl := "http://" + target + ":" + port + "/hit"
		fmt.Fprintf(w, "Started hit : %s, %d times", targetUrl, counter)
		for i := 0; i <= counter; i++ {
			start := time.Now()
			_, err := http.Get(targetUrl)
			t := time.Now()
			elapsed := t.Sub(start)
			if err != nil {
				log.Fatalf("Fail to reach %s - %s", targetUrl, err)
			}
			log.Printf("#%d - Duration %s", i, elapsed.String())
			perf.With(prometheus.Labels{"target": targetUrl}).Set(elapsed.Seconds())
			time.Sleep(1 * time.Second)
		}
	})

	log.Fatal(http.ListenAndServe(*addr, nil))
}
