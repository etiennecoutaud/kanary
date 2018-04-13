/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clientset

import (
	glog "github.com/golang/glog"
	discovery "k8s.io/client-go/discovery"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
	kopsinternalversion "k8s.io/kops/pkg/client/clientset_generated/clientset/typed/kops/internalversion"
	kopsv1alpha1 "k8s.io/kops/pkg/client/clientset_generated/clientset/typed/kops/v1alpha1"
	kopsv1alpha2 "k8s.io/kops/pkg/client/clientset_generated/clientset/typed/kops/v1alpha2"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	Kops() kopsinternalversion.KopsInterface
	KopsV1alpha1() kopsv1alpha1.KopsV1alpha1Interface
	KopsV1alpha2() kopsv1alpha2.KopsV1alpha2Interface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*discovery.DiscoveryClient
	kops         *kopsinternalversion.KopsClient
	kopsV1alpha1 *kopsv1alpha1.KopsV1alpha1Client
	kopsV1alpha2 *kopsv1alpha2.KopsV1alpha2Client
}

// Kops retrieves the KopsClient
func (c *Clientset) Kops() kopsinternalversion.KopsInterface {
	return c.kops
}

// KopsV1alpha1 retrieves the KopsV1alpha1Client
func (c *Clientset) KopsV1alpha1() kopsv1alpha1.KopsV1alpha1Interface {
	return c.kopsV1alpha1
}

// KopsV1alpha2 retrieves the KopsV1alpha2Client
func (c *Clientset) KopsV1alpha2() kopsv1alpha2.KopsV1alpha2Interface {
	return c.kopsV1alpha2
}

// Discovery retrieves the DiscoveryClient
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// NewForConfig creates a new Clientset for the given config.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.kops, err = kopsinternalversion.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.kopsV1alpha1, err = kopsv1alpha1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.kopsV1alpha2, err = kopsv1alpha2.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfig(&configShallowCopy)
	if err != nil {
		glog.Errorf("failed to create the DiscoveryClient: %v", err)
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	var cs Clientset
	cs.kops = kopsinternalversion.NewForConfigOrDie(c)
	cs.kopsV1alpha1 = kopsv1alpha1.NewForConfigOrDie(c)
	cs.kopsV1alpha2 = kopsv1alpha2.NewForConfigOrDie(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClientForConfigOrDie(c)
	return &cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.kops = kopsinternalversion.New(c)
	cs.kopsV1alpha1 = kopsv1alpha1.New(c)
	cs.kopsV1alpha2 = kopsv1alpha2.New(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}