package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Kanary describes a Kanary deployment.
type Kanary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KanarySpec   `json:"spec"`
	Status KanaryStatus `json:"status"`
}

// KanarySpec is the spec for a Kanary resource
type KanarySpec struct {
	Destination string        `json:"destination"`
	Routes      []KanaryRoute `json:"routes"`
}

// KanaryRoute route
type KanaryRoute struct {
	Backend KanaryService `json:"backend"`
	Weight  int           `json:"weight"`
}

// KanaryService basic information to build a service
type KanaryService struct {
	ServiceName string             `json:"servicename"`
	ServicePort intstr.IntOrString `json:"serviceport"`
}

// KanaryStatus for kanary
type KanaryStatus struct {
	DestinationStatus DestinationStatus    `json:"destinationStatus"`
	EndpointStatus    []KanaryEndpointList `json:"endpointStatuses"`
}

// //DestinationStatus for kanary
type DestinationStatus struct {
	ProxyName   string `json:"proxyName"`
	ServiceName string `json:"serviceName"`
	ConfigName  string `json:"configName"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KanaryList is a list of Kanary resources
type KanaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Kanary `json:"items"`
}

type KanaryEndpointList struct {
	ServiceName string
	Weight      int
	Ips         []string
}

// Check current status to determined if haproxy deployment need to be create
func (ky *Kanary) HAProxyCreationNeeded() bool {
	return ky.Status.DestinationStatus.ProxyName == ""
}

//GetServiceNameFromRoute get generated service name from route name
// func (ky *Kanary) GetServiceNameFromRoute(routeName string) (string, error) {
// 	for _, routeStatus := range ky.Status.RoutesStatus {
// 		if routeName == routeStatus.RouteName {
// 			return routeStatus.ServiceName, nil
// 		}
// 	}
// 	return "", errors.New("Associated service to route: " + routeName + " not found")
// }

//GetNewBackend list all new backend
// func (ky *Kanary) GetNewBackend() []KanaryRoute {
// 	var newBackends []KanaryRoute

// 	for _, backend := range ky.Spec.Routes {
// 		if !ky.routeExistInStatus(backend.Backend.Name) {
// 			newBackends = append(newBackends, backend)
// 		}
// 	}
// 	return newBackends
// }

//GetDeletedBackend get all delete backend
// func (ky *Kanary) GetDeletedBackend() []RouteStatus {
// 	var deletedBackends []RouteStatus

// 	for _, backend := range ky.Status.RoutesStatus {
// 		if !ky.routeExistInSpec(backend.RouteName) {
// 			deletedBackends = append(deletedBackends, backend)
// 		}
// 	}
// 	return deletedBackends
// }

// func (ky *Kanary) routeExistInStatus(routeName string) bool {
// 	for _, routeStatus := range ky.Status.RoutesStatus {
// 		if routeName == routeStatus.RouteName {
// 			return true
// 		}
// 	}
// 	return false
// }

// func (ky *Kanary) routeExistInSpec(routeName string) bool {
// 	for _, routeSpec := range ky.Spec.Routes {
// 		if routeName == routeSpec.Backend.Name {
// 			return true
// 		}
// 	}
// 	return false
// }

// //DeleteRouteStatus getNewRouteStatus
// func (ky *Kanary) DeleteRouteStatus(routeName string) []RouteStatus {
// 	var result []RouteStatus
// 	for _, routeStatus := range ky.Status.RoutesStatus {
// 		if routeStatus.RouteName != routeName {
// 			result = append(result, routeStatus)
// 		}
// 	}
// 	return result
// }
