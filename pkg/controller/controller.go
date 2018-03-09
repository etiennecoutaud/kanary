package controller

import (
	"fmt"
	"strconv"
	"time"

	"reflect"

	kanaryv1 "github.com/etiennecoutaud/kanary/pkg/apis/kanary/v1"
	clientset "github.com/etiennecoutaud/kanary/pkg/client/clientset/versioned"
	kanaryscheme "github.com/etiennecoutaud/kanary/pkg/client/clientset/versioned/scheme"
	informers "github.com/etiennecoutaud/kanary/pkg/client/informers/externalversions"
	listers "github.com/etiennecoutaud/kanary/pkg/client/listers/kanary/v1"
	"github.com/golang/glog"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	// controllerAgentName is the name in event sources
	controllerAgentName = "kanary-controller"

	// SuccessSynced is used as part of the Event 'reason' when a Database is synced
	SuccessSynced = "Synced"

	// KanaryCreated is used to create events when a pod has been deleted
	KanaryCreated = "Kanary"

	// PodSelectingError is used to store events regarding Pod selecting issues
	PodSelectingError = "Pod Selecting Error"

	// PodListingEmpty is used to store events regarding Pod listing that return no pods
	PodListingEmpty = "Pod List Empty"

	// PodListingError is used to store events regarding Pod listing issues
	PodListingError = "Pod Listing Error"

	// PodDeletingError is used o store events regarding Pod deleting issue
	PodDeletingError = "Pod Deleting Error"

	// MessageResourceSynced is the message used for an Event fired when a Database
	// is synced successfully
	MessageResourceSynced = "Kanary synced successfully"

	ErrWeightRoutes    = "ErrWeightRoutes"
	ErrWeightRoutesMsg = "Route weight sum must equal 100"
)

// Controller is the controller implementation for kanary resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	kanaryclientset clientset.Interface

	kanaryLister listers.KanaryLister
	kanarySynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new kanary controller
func NewController(
	kubeclientset kubernetes.Interface,
	kanaryclientset clientset.Interface,
	kanaryInformerFactory informers.SharedInformerFactory) *Controller {

	kanaryInformer := kanaryInformerFactory.Kanary().V1().Kanaries()

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	kanaryscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:   kubeclientset,
		kanaryclientset: kanaryclientset,
		kanaryLister:    kanaryInformer.Lister(),
		kanarySynced:    kanaryInformer.Informer().HasSynced,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DBs"),
		recorder:        recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when KanaryRule resources change
	kanaryInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueKY,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueKY(new)
		},
		DeleteFunc: controller.enqueueKY,
	})
	return controller
}

// enqueueKY takes a Kanary resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Db.
func (c *Controller) enqueueKY(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting Kanary controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.kanarySynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process Database resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Database resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Kanary resource with this namespace/name
	ky, err := c.kanaryLister.Kanaries(namespace).Get(name)
	if err != nil {
		// The Kanary resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			// Delete All stack Service + Pod + Service
			// Add annotation with generator name
			glog.V(2).Info(fmt.Sprintf("Kanary %s no longer exists", key))
			return nil
		}
		return err
	}

	// Check that weight total is 100
	weightTotal := 0
	for _, route := range ky.Spec.Routes {
		weightTotal += route.Weight
	}
	if weightTotal != 100 {
		runtime.HandleError(fmt.Errorf("%s: Route weight sum must equal 100 currently %d", key, weightTotal))
		return nil
	}

	// Newly Ressource create the whole stack
	if reflect.DeepEqual(ky.Status, kanaryv1.KanaryStatus{}) {
		// Create destination Service
		service, err := c.kubeclientset.CoreV1().Services(namespace).Create(newService(ky, &ky.Spec.Destination, true, 100))
		if err != nil {
			runtime.HandleError(fmt.Errorf("Fail to create service %s", err))
			return nil
		}
		ky, err = c.updateStatusService(ky, service, true)
		if err != nil {
			return err
		}
		c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateSvcMsg(service.Name))
		// Create each backend service
		var backend []*corev1.Service
		for _, svc := range ky.Spec.Routes {
			service, err = c.kubeclientset.CoreV1().Services(namespace).Create(newService(ky, &svc.Backend, false, svc.Weight))
			if err != nil {
				runtime.HandleError(fmt.Errorf("Fail to create service %s", err))
				return nil
			}
			ky, err = c.updateStatusService(ky, service, false)
			if err != nil {
				runtime.HandleError(fmt.Errorf("Fail to update status %s, %s", service.Name, err))
				return nil
			}
			backend = append(backend, service)
			c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateSvcMsg(service.Name))
		}
		// Create configMap to hold haproxy configuration based on generated service
		hpc := newHAProxyController(backend)
		config, err := hpc.generateConfig()
		if err != nil {
			runtime.HandleError(fmt.Errorf("Fail to generate haproxy config %s", err))
			return nil
		}
		cm, err := c.kubeclientset.CoreV1().ConfigMaps(namespace).Create(newConfigMap(ky, config))
		if err != nil {
			runtime.HandleError(fmt.Errorf("Fail to create configmap %s", err))
			return nil
		}
		ky, err = c.updateStatusConfigMap(ky, cm)
		if err != nil {
			runtime.HandleError(fmt.Errorf("Fail to update status %s, %s", cm.Name, err))
			return nil
		}
		c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateCMMsg(cm.Name))

		deployment, err := c.kubeclientset.AppsV1().Deployments(namespace).Create(newDeployementHA(ky, cm.Name))
		if err != nil {
			runtime.HandleError(fmt.Errorf("Fail to create deployment %s", err))
			return nil
		}
		ky, err = c.updateStatusDeployment(ky, deployment)
		if err != nil {
			runtime.HandleError(fmt.Errorf("Fail to update status %s, %s", deployment.Name, err))
			return nil
		}
		c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateDeploymentMsg(deployment.Name))
	}

	c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func newService(owner *kanaryv1.Kanary, svc *kanaryv1.KanaryService, destination bool, weight int) *corev1.Service {
	var generatedName string
	var port int32
	var labelSelector map[string]string

	if destination {
		generatedName = svc.Name
		port = 80
		labelSelector = map[string]string{"proxy": owner.Name + "-haproxy"}
	} else {
		generatedName = svc.Name + generateName()
		port = svc.Port
		labelSelector = svc.PodSelector.MatchLabels
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        generatedName,
			Namespace:   owner.Namespace,
			Annotations: map[string]string{"routeName": svc.Name, "weight": strconv.Itoa(weight)},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, schema.GroupVersionKind{
					Group:   kanaryv1.SchemeGroupVersion.Group,
					Version: kanaryv1.SchemeGroupVersion.Version,
					Kind:    "Kanary",
				}),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: port,
				},
			},
			Selector: labelSelector,
			Type:     corev1.ServiceTypeClusterIP,
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

func newDeployementHA(owner *kanaryv1.Kanary, cmName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        owner.Name + "-haproxy",
			Namespace:   owner.Namespace,
			Labels:      map[string]string{"proxy": owner.Name + "-haproxy"},
			Annotations: map[string]string{"configmap": cmName, "configmapVersion": strconv.Itoa(1)},
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
				MatchLabels: map[string]string{"haproxy": owner.Name + "-haproxy"},
			},
			Template: newPodTemplate(owner.Name, cmName),
		},
	}
}

func newPodTemplate(ownerName string, cmName string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"haproxy": ownerName + "-haproxy"},
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

func (c Controller) updateStatusService(ky *kanaryv1.Kanary, svc *corev1.Service, destination bool) (*kanaryv1.Kanary, error) {
	kycopy := ky.DeepCopy()

	if destination {
		kycopy.Status.DestinationStatus.ServiceName = svc.GetName()
	} else {
		status := kanaryv1.RouteStatus{
			RouteName:   svc.GetAnnotations()["routeName"],
			ServiceName: svc.Name,
		}
		kycopy.Status.RoutesStatus = append(ky.Status.RoutesStatus, status)
	}
	newky, err := c.kanaryclientset.KanaryV1().Kanaries(ky.Namespace).Update(kycopy)
	return newky, err
}

func (c Controller) updateStatusConfigMap(ky *kanaryv1.Kanary, cm *corev1.ConfigMap) (*kanaryv1.Kanary, error) {
	kycopy := ky.DeepCopy()
	kycopy.Status.DestinationStatus.ConfigName = cm.Name
	newky, err := c.kanaryclientset.KanaryV1().Kanaries(ky.Namespace).Update(kycopy)
	return newky, err
}

func (c Controller) updateStatusDeployment(ky *kanaryv1.Kanary, deployment *appsv1.Deployment) (*kanaryv1.Kanary, error) {
	kycopy := ky.DeepCopy()
	kycopy.Status.DestinationStatus.ProxyName = deployment.Name
	newky, err := c.kanaryclientset.KanaryV1().Kanaries(ky.Namespace).Update(kycopy)
	return newky, err
}
