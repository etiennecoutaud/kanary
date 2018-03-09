package controller

import (
	"bytes"
	"strconv"

	"text/template"

	corev1 "k8s.io/api/core/v1"
)

const (
	configFileName = "haproxy.cfg"
	configTemplate = `
	global
      log /dev/log local2 info
      pidfile /var/run/haproxy.pid
      ulimit-n        65536

    defaults
      mode    http
      option  httplog
      option  dontlognull
      option  forwardfor
      option  contstats
      option  http-server-close
      option log-health-checks
      retries 3
      option  redispatch
      timeout connect  5000
      timeout client  10000
      timeout server  10000

      log global
      log-format {"type":"haproxy","timestamp":%Ts,"http_status":%ST,"http_request":"%r","remote_addr":"%ci","bytes_read":%B,"upstream_addr":"%si","backend_name":"%b","retries":%rc,"bytes_uploaded":%U,"upstream_response_time":"%Tr","upstream_connect_time":"%Tc","session_duration":"%Tt","termination_state":"%ts"}

      stats enable
      stats uri		/monitor
      stats refresh	5s
      option httpchk	GET /status
      retries		5

    frontend http-in
      bind :80
      default_backend kanary

    backend kanary
	  {{range .Backend -}}
      server {{.Name}} {{.Name}}:{{.Port}} weight {{.Weight}}
	  {{end}}`
)

//SimpleService simple struct to represent kanaryService
type SimpleService struct {
	Name   string
	Port   string
	Weight string
}

//HAProxyController to manage configFile
type HAProxyController struct {
	Backend []*SimpleService
}

func newHAProxyController(svcList []*corev1.Service) *HAProxyController {
	var backend []*SimpleService

	for _, svc := range svcList {
		simpleSvc := &SimpleService{
			Name:   svc.Name,
			Port:   strconv.Itoa(int(svc.Spec.Ports[0].Port)),
			Weight: svc.ObjectMeta.Annotations["weight"],
		}
		backend = append(backend, simpleSvc)
	}
	return &HAProxyController{
		Backend: backend,
	}
}

func (hpc *HAProxyController) generateConfig() (string, error) {
	var ouput bytes.Buffer

	tmpl := template.New(configFileName)
	tmpl, err := tmpl.Parse(configTemplate)
	if err != nil {
		return "", err
	}

	err = tmpl.Execute(&ouput, hpc)
	if err != nil {
		return "", err
	}

	return ouput.String(), nil
}
