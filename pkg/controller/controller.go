package controller

import (
	"fmt"
	"strconv"
	"time"

	"encoding/json"

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
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	maxRetries = 15
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
	epLister     corelister.EndpointsLister

	kanaryListerSynced cache.InformerSynced
	epListerSynced     cache.InformerSynced

	// kanary that needto be synced
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new kanary controller
func NewController(
	kubeclientset kubernetes.Interface,
	kanaryclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	kanaryInformerFactory informers.SharedInformerFactory) *Controller {

	kanaryInformer := kanaryInformerFactory.Kanary().V1().Kanaries()
	epInformer := kubeInformerFactory.Core().V1().Endpoints()

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
		kubeclientset:      kubeclientset,
		kanaryclientset:    kanaryclientset,
		kanaryLister:       kanaryInformer.Lister(),
		epLister:           epInformer.Lister(),
		kanaryListerSynced: kanaryInformer.Informer().HasSynced,
		epListerSynced:     epInformer.Informer().HasSynced,
		workqueue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "kanary"),
		recorder:           recorder,
	}

	kanaryInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addKanary,
		UpdateFunc: controller.updateKanary,
		DeleteFunc: controller.deleteKanary,
	})

	epInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addEndpoint,
		UpdateFunc: controller.updateEndpoint,
		DeleteFunc: controller.deleteEndpoint,
	})

	glog.Info("Setting up event handlers")
	return controller
}

// enqueueKY takes a Kanary resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Db.
func (c *Controller) enqueueKanary(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting Kanary controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	//if ok := cache.WaitForCacheSync(stopCh, c.kanaryListerSynced, c.epListerSynced); !ok {
	if ok := cache.WaitForCacheSync(stopCh, c.kanaryListerSynced, c.epListerSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process Database resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *Controller) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.workqueue.Get()
	if quit {
		return false
	}
	defer c.workqueue.Done(key)
	err := c.syncKanary(key.(string))
	c.handleErr(err, key)

	return true
}

func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		c.workqueue.Forget(key)
		return
	}

	if c.workqueue.NumRequeues(key) < maxRetries {
		glog.V(2).Infof("Error syncing kanary %v: %v", key, err)
		c.workqueue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	glog.V(2).Infof("Dropping kanary %q out of the queue: %v", key, err)
	c.workqueue.Forget(key)
}

func (c *Controller) addKanary(obj interface{}) {
	ky := obj.(*kanaryv1.Kanary)
	glog.V(4).Infof("Adding Kanary %s", ky.Name)
	c.enqueueKanary(ky)
}

func (c *Controller) updateKanary(old interface{}, cur interface{}) {
	//oldKy := old.(*kanaryv1.Kanary)
	curKy := cur.(*kanaryv1.Kanary)

	// if diff log diff
	c.enqueueKanary(curKy)
}

func (c *Controller) deleteKanary(obj interface{}) {
	ky := obj.(*kanaryv1.Kanary)
	glog.V(4).Infof("Deleting Kanary %s", ky.Name)
	err := c.deleteKyRefInEp(ky)
	if err != nil {
		glog.Error(err)
	}
}

func (c *Controller) addEndpoint(obj interface{}) {
	// Check if endpoint is in pool update pool if need
}

func (c *Controller) updateEndpoint(obj interface{}, cur interface{}) {
	// Check if url has change, change haproxy config
	ep := obj.(*corev1.Endpoints)
	var kyRef []string
	if value, ok := ep.ObjectMeta.Annotations["kanaryRef"]; ok {
		if err := json.Unmarshal([]byte(value), &kyRef); err != nil {
			glog.Error(err)
		}
		if len(kyRef) == 0 {
			return
		}
		for _, kyName := range kyRef {
			ky, err := c.kanaryLister.Kanaries(ep.Namespace).Get(kyName)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to get reference kanary: %s", kyName))
				return
			}
			glog.V(4).Infof("Endpoint is refered by %s", kyName)
			c.enqueueKanary(ky)
		}
	}
}

func (c *Controller) deleteEndpoint(obj interface{}) {
	// Remove endpoint from ressource
}

// Sync loop for Kanary resources
func (c *Controller) syncKanary(key string) error {

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
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

	// Build expected endpoint status to compare with actual
	expectedEndpointStatus, err := c.buildExpectedEndpointStatus(ky)
	if err != nil {
		glog.V(2).Info(fmt.Sprintf("Fail to build expected endpoint status : %s", err))
		return nil
	}
	// Check for diff update if different
	if !deepEqualEndpointStatusList(expectedEndpointStatus, ky.Status.EndpointStatus) {
		haProxyController := newHAProxyController(expectedEndpointStatus)
		haConfig, err := haProxyController.generateConfig()
		if err != nil {
			glog.V(2).Info(fmt.Sprintf("Fail to build HAproxy config : %s", err))
			return nil
		}
		var cm *corev1.ConfigMap
		if ky.Status.DestinationStatus.ConfigName == "" {
			cm, err = c.kubeclientset.CoreV1().ConfigMaps(namespace).Create(newConfigMap(ky, haConfig))
			if err != nil {
				glog.V(2).Info(fmt.Sprintf("Fail to create HAProxy configMap : %s", err))
				return nil
			}
			c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateCMMsg(cm.Name))
		} else {
			cm, err = c.kubeclientset.CoreV1().ConfigMaps(namespace).Update(newConfigMap(ky, haConfig))
			if err != nil {
				glog.V(2).Info(fmt.Sprintf("Fail to update HAProxy configMap : %s", err))
				return nil
			}
			err = c.performRollingUpdate(ky.Status.DestinationStatus.ProxyName, ky.Namespace)
			if err != nil {
				glog.Errorf("Fail to perform rolling update, %s", err)
				return nil
			}
			c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successUpdateCMMsg(cm.Name))

		}

		ky, err = c.updateStatusConfigMap(ky, cm)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("Fail to update configmap ref status %s, %s", cm.Name, err))
			return nil
		}
		ky, err = c.updateStatusEndpoint(ky, expectedEndpointStatus)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("Fail to update endpoint status '%v', %s", expectedEndpointStatus, err))
			return nil
		}

		// Add Kanary owner ref to follow update
		for _, epRef := range ky.Status.EndpointStatus {
			ep, err := c.epLister.Endpoints(namespace).Get(epRef.ServiceName)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to find endpoint %s", err))
				return nil
			}
			newEp, err := addKanaryRefInEp(ep, ky.Name)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to add kanaryRef %s", err))
				return nil
			}
			_, err = c.kubeclientset.CoreV1().Endpoints(namespace).Update(newEp)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to update endpoint %s", err))
				return nil
			}
		}

	}

	// Manage Pod
	if ky.HAProxyCreationNeeded() {
		deployment, err := c.kubeclientset.AppsV1().Deployments(namespace).Create(newDeployementHA(ky))
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("Fail to create deployment %s", err))
			return nil
		}
		ky, err = c.updateStatusDeployment(ky, deployment)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("Fail to update status %s, %s", deployment.Name, err))
			return nil
		}
		c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateDeploymentMsg(deployment.Name))
	}

	// HAProxy service create or update ?
	if ky.Spec.Destination != ky.Status.DestinationStatus.ServiceName {
		// Create destination Service
		var service *corev1.Service
		if ky.Status.DestinationStatus.ServiceName == "" {
			service, err = c.kubeclientset.CoreV1().Services(namespace).Create(newService(ky))
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to create service %s", err))
				return nil
			}
			c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successCreateSvcMsg(service.Name))
		} else {
			err = c.kubeclientset.CoreV1().Services(namespace).Delete(ky.Status.DestinationStatus.ServiceName, nil)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to delete service %s", err))
				return nil
			}
			service, err = c.kubeclientset.CoreV1().Services(namespace).Create(newService(ky))
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("Fail to update service %s", err))
				return nil
			}
			c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, successUpdateSvcMsg(service.Name))
		}
		ky, err = c.updateStatusService(ky, service)
		if err != nil {
			return err
		}
	}

	c.recorder.Event(ky, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

//TODO: Add test here
func (c *Controller) buildExpectedEndpointStatus(ky *kanaryv1.Kanary) ([]kanaryv1.KanaryEndpointList, error) {
	var kyEpList []kanaryv1.KanaryEndpointList

	for _, route := range ky.Spec.Routes {
		ep, err := c.epLister.Endpoints(ky.Namespace).Get(route.Backend.ServiceName)
		if err != nil {
			return nil, err
		}
		servicePortInt32 := verifyEndpointPortExist(ep, route.Backend.ServicePort)
		if servicePortInt32 == -1 {
			return nil, errors.NewNotFound(corev1.Resource("endpoints"), route.Backend.ServicePort.StrVal)
		}
		var kyEp kanaryv1.KanaryEndpointList
		kyEp.ServiceName = route.Backend.ServiceName
		kyEp.Weight = route.Weight
		for _, subset := range ep.Subsets {
			for _, addr := range subset.Addresses {
				fullIP := addr.IP + ":" + strconv.Itoa(int(servicePortInt32))
				kyEp.Ips = append(kyEp.Ips, fullIP)
			}
		}
		kyEpList = append(kyEpList, kyEp)
	}

	return kyEpList, nil
}

func (c Controller) updateStatusEndpoint(ky *kanaryv1.Kanary, epList []kanaryv1.KanaryEndpointList) (*kanaryv1.Kanary, error) {
	kycopy := ky.DeepCopy()
	kycopy.Status.EndpointStatus = epList
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

func (c Controller) updateStatusService(ky *kanaryv1.Kanary, svc *corev1.Service) (*kanaryv1.Kanary, error) {
	kycopy := ky.DeepCopy()
	kycopy.Status.DestinationStatus.ServiceName = svc.Name
	newky, err := c.kanaryclientset.KanaryV1().Kanaries(ky.Namespace).Update(kycopy)
	return newky, err
}

func newService(owner *kanaryv1.Kanary) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Spec.Destination,
			Namespace: owner.Namespace,
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
					Port: 80,
				},
			},
			Selector: map[string]string{"proxy": owner.Name + "-haproxy"},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func addKanaryRefInEp(ep *corev1.Endpoints, owner string) (*corev1.Endpoints, error) {
	epCopy := ep.DeepCopy()

	var kanaryRefRange []string
	var kanaryRefString []byte

	kanaryRef := []byte(epCopy.Annotations["kanaryRef"])
	if len(kanaryRef) != 0 {
		err := json.Unmarshal(kanaryRef, &kanaryRefRange)
		if err != nil {
			glog.Errorf("Fail to Unmarshall string %s", kanaryRef)
			return nil, err
		}
	}
	if !containsString(kanaryRefRange, owner) {
		kanaryRefRange = append(kanaryRefRange, owner)
	}
	kanaryRefString, err := json.Marshal(kanaryRefRange)
	if err != nil {
		glog.Errorf("Fail to Marshal slice '%v'", kanaryRefRange)
		return nil, err
	}
	if len(epCopy.Annotations) == 0 {
		epCopy.Annotations = map[string]string{}
	}
	epCopy.Annotations["kanaryRef"] = string(kanaryRefString[:])
	glog.Infof("Struct : '%v'", epCopy)
	return epCopy, nil
}

func (c *Controller) deleteKyRefInEp(ky *kanaryv1.Kanary) error {
	var kyRef []string
	for _, epStatus := range ky.Status.EndpointStatus {
		ep, err := c.epLister.Endpoints(ky.Namespace).Get(epStatus.ServiceName)
		if err != nil {
			return err
		}
		epCopy := ep.DeepCopy()
		if err = json.Unmarshal([]byte(epCopy.Annotations["kanaryRef"]), &kyRef); err != nil {
			return err
		}
		indexElt := getEltToDeleteIndex(kyRef, ky.Name)
		kyRef = append(kyRef[:indexElt], kyRef[indexElt+1:]...)
		kyRefString, err := json.Marshal(kyRef)
		if err != nil {
			return err
		}
		epCopy.Annotations["kanaryRef"] = string(kyRefString[:])
		_, err = c.kubeclientset.CoreV1().Endpoints(ky.Namespace).Update(epCopy)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) performRollingUpdate(deploymentName string, ns string) error {
	deploy, err := c.kubeclientset.AppsV1().Deployments(ns).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	deployCopy := deploy.DeepCopy()
	revision, err := strconv.Atoi(deployCopy.Spec.Template.Annotations["kanary.revision"])
	if err != nil {
		glog.V(2).Info("Fail to convert kanary.revision, set to 0")
		revision = 0
	}
	deployCopy.Spec.Template.Annotations["kanary.revision"] = strconv.Itoa(revision + 1)
	_, err = c.kubeclientset.AppsV1().Deployments(ns).Update(deployCopy)
	return err
}
