package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	clientset "github.com/etiennecoutaud/kanary/pkg/client/clientset/versioned"
	informers "github.com/etiennecoutaud/kanary/pkg/client/informers/externalversions"
	controller "github.com/etiennecoutaud/kanary/pkg/controller"
	route "github.com/etiennecoutaud/kanary/pkg/route"
	signals "github.com/etiennecoutaud/kanary/pkg/signal"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"goji.io"
	"goji.io/pat"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kuberconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	master      = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	version     = "No version"
	timestamp   = "0.0"
)

func debugHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			log.Printf("%s %s", r.Method, r.URL)
			next.ServeHTTP(w, r)
		})
}

func main() {
	flag.Parse()
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(*master, *kuberconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}
	kanaryClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kanary clientset: %v", err)
	}

	kanaryInformerFactory := informers.NewSharedInformerFactory(kanaryClient, time.Second*30)

	controller := controller.NewController(kubeClient, kanaryClient, kanaryInformerFactory)

	go kanaryInformerFactory.Start(stopCh)

	mux := goji.NewMux()
	mux.Use(debugHandler)
	mux.HandleFunc(pat.Get("/healthz"), route.Healthz)
	mux.Handle(pat.Get("/metrics"), promhttp.Handler())

	go http.ListenAndServe(":8080", mux)

	if err = controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}

}
