package controller

import (
	"bytes"
	"strconv"

	"text/template"

	kanaryv1 "github.com/etiennecoutaud/kanary/pkg/apis/kanary/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	configFileName = "haproxy.cfg"
	configTemplate = `
	global
      log /dev/log local2 info
      pidfile /var/run/haproxy.pid
      ulimit-n        65536

    defaults
      mode    tcp
      option  tcplog
      option  dontlognull
      option  contstats
      option log-health-checks
      retries 3
      timeout connect  5000
      timeout client  10000
      timeout server  10000

      log global

      stats enable
      stats uri		/monitor
      stats refresh	5s
      option httpchk	GET /status
      retries		5

    frontend in
      bind :80
      default_backend kanary

    backend kanary
	  {{range .Endpoints -}}
      server {{.Name}} {{.IP}} weight {{.Weight}}
	  {{end}}`
)

//HAConfigBackend struct use to represent each backend line in conf
type HAConfigBackend struct {
	Name   string
	Weight float32
	IP     string
}

//HAProxyController to manage configFile
type HAProxyController struct {
	Endpoints []HAConfigBackend
}

func newHAProxyController(epLists []kanaryv1.KanaryEndpointList) HAProxyController {

	var endpoints []HAConfigBackend

	for _, kyEp := range epLists {
		for i, ip := range kyEp.Ips {
			haConfig := HAConfigBackend{
				Name:   kyEp.ServiceName + "-" + strconv.Itoa(i),
				Weight: float32(kyEp.Weight) / float32(len(kyEp.Ips)),
				IP:     ip,
			}
			endpoints = append(endpoints, haConfig)
		}
	}
	return HAProxyController{
		Endpoints: endpoints,
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

func newDeployementHA(owner *kanaryv1.Kanary) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        owner.Name + "-haproxy",
			Namespace:   owner.Namespace,
			Labels:      map[string]string{"proxy": owner.Name + "-haproxy"},
			Annotations: map[string]string{"configmap": owner.Status.DestinationStatus.ConfigName, "configmapVersion": strconv.Itoa(1)},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, schema.GroupVersionKind{
					Group:   kanaryv1.SchemeGroupVersion.Group,
					Version: kanaryv1.SchemeGroupVersion.Version,
					Kind:    "Kanary",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"proxy": owner.Name + "-haproxy"},
			},
			Template: newPodTemplate(owner.Name, owner.Status.DestinationStatus.ConfigName),
		},
	}
}

func newConfigMap(owner *kanaryv1.Kanary, config string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-config",
			Namespace: owner.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, schema.GroupVersionKind{
					Group:   kanaryv1.SchemeGroupVersion.Group,
					Version: kanaryv1.SchemeGroupVersion.Version,
					Kind:    "Kanary",
				}),
			},
		},
		Data: map[string]string{configFileName: config},
	}
}

func newPodTemplate(ownerName string, cmName string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"proxy": ownerName + "-haproxy"},
			Annotations: map[string]string{"kanary.revision": "1"},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "ha-cfg",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cmName,
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "haproxy",
					Image: "etiennecoutaud/haproxy:latest",
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
						{
							Name:          "monitor",
							ContainerPort: 9000,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "ha-cfg",
							MountPath: "/usr/local/etc/haproxy",
							ReadOnly:  true,
						},
					},
				},
				{
					Name:  "metrics-sidecar",
					Image: "etiennecoutaud/metrics-sidecar:latest",
					Ports: []corev1.ContainerPort{
						{
							Name:          "metrics",
							ContainerPort: 9101,
						},
					},
				},
			},
		},
	}
}
