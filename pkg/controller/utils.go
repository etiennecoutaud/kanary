package controller

import (
	"math/rand"
	"time"

	kanaryv1 "github.com/etiennecoutaud/kanary/pkg/apis/kanary/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func String(length int) string {
	return StringWithCharset(length, charset)
}

func generateName() string {
	return String(6)
}

func successCreateSvcMsg(name string) string {
	return "Create " + name + " service successfully"
}

func successUpdateSvcMsg(name string) string {
	return "Update " + name + " service successfully"
}

func successCreateCMMsg(name string) string {
	return "Create " + name + " configmap successfully"
}

func successUpdateCMMsg(name string) string {
	return "Update " + name + " configmap successfully and rolling update performed"
}

func successCreateDeploymentMsg(name string) string {
	return "Create " + name + " deployment successfully"
}

func successDeleteSvcMsg(name string) string {
	return "Delete " + name + " service successfully"
}

func verifyEndpointPortExist(ep *corev1.Endpoints, port intstr.IntOrString) int32 {
	for _, subset := range ep.Subsets {
		for _, p := range subset.Ports {
			if p.Name == port.StrVal || p.Port == port.IntVal {
				return p.Port
			}
		}
	}
	return -1
}

func deepEqualEndpointStatusList(epListA []kanaryv1.KanaryEndpointList, epListB []kanaryv1.KanaryEndpointList) bool {
	if len(epListA) == len(epListB) && len(epListA) == 0 {
		return true
	}
	if len(epListA) != len(epListB) {
		return false
	}
	result := true
	for _, epStatusA := range epListA {
		result = result && hasEndpointStatus(epListB, epStatusA)
	}
	return result
}

func hasEndpointStatus(slice []kanaryv1.KanaryEndpointList, elem kanaryv1.KanaryEndpointList) bool {
	for _, el := range slice {
		if deepEqualEndpointStatus(elem, el) {
			return true
		}
	}
	return false
}

func deepEqualEndpointStatus(epStatusA kanaryv1.KanaryEndpointList, epStatusB kanaryv1.KanaryEndpointList) bool {
	result := true

	result = result && (epStatusA.ServiceName == epStatusB.ServiceName)
	result = result && (epStatusA.Weight == epStatusB.Weight)

	for _, ipA := range epStatusA.Ips {
		result = result && containsString(epStatusB.Ips, ipA)
	}

	return result
}

func containsString(slice []string, elem string) bool {
	for _, el := range slice {
		if el == elem {
			return true
		}
	}
	return false
}

func getEltToDeleteIndex(slice []string, elt string) int {
	for i, e := range slice {
		if e == elt {
			return i
		}
	}
	return -1
}
