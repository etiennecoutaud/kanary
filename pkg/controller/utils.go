package controller

import (
	"math/rand"
	"time"

	kanaryv1 "github.com/etiennecoutaud/kanary/pkg/apis/kanary/v1"
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

func successCreateCMMsg(name string) string {
	return "Create " + name + " configmap successfully"
}

func successCreateDeploymentMsg(name string) string {
	return "Create " + name + " deployment successfully"
}

func statusRouteExist(ky *kanaryv1.Kanary, name string) bool {
	if len(ky.Status.RoutesStatus) == 0 {
		return false
	}
	for _, route := range ky.Status.RoutesStatus {
		if route.ServiceName == name {
			return true
		}
	}
	return false
}
