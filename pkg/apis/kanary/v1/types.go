package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Destination KanaryService `json:"destination,omitempty" protobuf:"bytes,2,opt,name=destination"`
	Routes      []KanaryRoute `json:"route"`
}

// KanaryRoute route
type KanaryRoute struct {
	Backend KanaryService `json:"backend"`
	Weight  int           `json:"weight"`
}

// KanaryService basic information to build a service
type KanaryService struct {
	Name        string                `json:"name"`
	PodSelector *metav1.LabelSelector `json:"podselector,omitempty" protobuf:"bytes,2,opt,name=podselector"`
	Port        int32                 `json:"port"`
}

// KanaryStatus for kanary
type KanaryStatus struct {
	DestinationStatus DestinationStatus `json:"destinationStatus"`
	RoutesStatus      []RouteStatus     `json:"routeStatuses"`
}

//DestinationStatus for kanary
type DestinationStatus struct {
	ProxyName   string `json:"proxyName"`
	ServiceName string `json:"serviceName"`
	ConfigName  string `json:"configName"`
}

// RouteStatus for kanary
type RouteStatus struct {
	RouteName   string `json:"name"`
	ServiceName string `json:"serviceName"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KanaryList is a list of Kanary resources
type KanaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Kanary `json:"items"`
}

// String simply prints ky id
// func (ky *Kanary) String() string {
// 	return fmt.Sprintf("KanaryRule (%s/%s)", ky.ObjectMeta.Namespace, ky.ObjectMeta.Name)
// }
