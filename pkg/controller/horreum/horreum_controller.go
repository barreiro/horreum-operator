package horreum

import (
	"context"
	"fmt"
	"reflect"

	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	logr "github.com/go-logr/logr"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_horreum")

// Add creates a new Horreum Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHorreum{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("horreum-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	if err = routev1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	// Watch for changes to primary resource Horreum
	err = c.Watch(&source.Kind{Type: &hyperfoilv1alpha1.Horreum{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Horreum
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hyperfoilv1alpha1.Horreum{},
	})
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hyperfoilv1alpha1.Horreum{},
	})
	err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hyperfoilv1alpha1.Horreum{},
	})
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hyperfoilv1alpha1.Horreum{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileHorreum implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileHorreum{}

// ReconcileHorreum reconciles a Horreum object
type ReconcileHorreum struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

type compareFunc func(interface{}, interface{}, logr.Logger) bool
type checkFunc func(interface{}) (bool, string, string)

var nocompare = func(interface{}, interface{}, logr.Logger) bool {
	return true
}
var nocheck = func(interface{}) (bool, string, string) {
	return true, "", ""
}

// Reconcile reads that state of the cluster for a Horreum object and makes changes based on the state read
// and what is in the Horreum.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHorreum) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Horreum")

	// Fetch the Horreum instance
	instance := &hyperfoilv1alpha1.Horreum{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.Status.Status != "Ready" {
		instance.Status.Status = "Ready"
		instance.Status.Reason = "Reconciliation succeeded."
		instance.Status.LastUpdate = metav1.Now()
	}

	dbAdminSecret := newSecret(instance, dbAdminSecret(instance))
	if err := ensureSame(r, instance, logger, dbAdminSecret, &corev1.Secret{}, nocompare,
		checkSecret(corev1.BasicAuthUsernameKey, corev1.BasicAuthPasswordKey)); err != nil {
		return reconcile.Result{}, err
	}
	appSecret := newSecret(instance, appUserSecret(instance))
	appSecret.StringData["dbsecret"] = generatePassword()
	if err := ensureSame(r, instance, logger, appSecret, &corev1.Secret{}, nocompare,
		checkSecret(corev1.BasicAuthUsernameKey, corev1.BasicAuthPasswordKey, "dbsecret")); err != nil {
		return reconcile.Result{}, err
	}
	keycloakAdminSecret := newSecret(instance, keycloakAdminSecret(instance))
	if err := ensureSame(r, instance, logger, keycloakAdminSecret, &corev1.Secret{}, nocompare,
		checkSecret(corev1.BasicAuthUsernameKey, corev1.BasicAuthPasswordKey)); err != nil {
		return reconcile.Result{}, err
	}
	keycloakDbSecret := newSecret(instance, keycloakDbSecret(instance))
	if err := ensureSame(r, instance, logger, keycloakDbSecret, &corev1.Secret{}, nocompare,
		checkSecret(corev1.BasicAuthUsernameKey, corev1.BasicAuthPasswordKey)); err != nil {
		return reconcile.Result{}, err
	}

	postgresPod := postgresPod(instance)
	postgresService := postgresService(instance)
	if instance.Spec.Postgres.ExternalHost != "" {
		if err := ensureDeleted(r, instance, postgresPod, &corev1.Pod{}); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureDeleted(r, instance, postgresService, &corev1.Service{}); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if err := ensureSame(r, instance, logger, postgresPod, &corev1.Pod{}, comparePods, checkPod); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureSame(r, instance, logger, postgresService, &corev1.Service{}, compareService, nocheck); err != nil {
			return reconcile.Result{}, err
		}
	}

	keycloakPod := keycloakPod(instance)
	keycloakService := keycloakService(instance)
	keycloakServiceSecure := keycloakServiceSecure(instance)
	keycloakRoute := keycloakRoute(instance)
	if instance.Spec.Keycloak.External {
		if err := ensureDeleted(r, instance, keycloakPod, &corev1.Pod{}); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureDeleted(r, instance, keycloakService, &corev1.Service{}); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureDeleted(r, instance, keycloakServiceSecure, &corev1.Service{}); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureDeleted(r, instance, keycloakRoute, &routev1.Route{}); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if err := ensureSame(r, instance, logger, keycloakPod, &corev1.Pod{}, comparePods, checkPod); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureSame(r, instance, logger, keycloakService, &corev1.Service{}, compareService, nocheck); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureSame(r, instance, logger, keycloakServiceSecure, &corev1.Service{}, compareService, nocheck); err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureSame(r, instance, logger, keycloakRoute, &routev1.Route{}, compareRoute, checkRoute); err != nil {
			return reconcile.Result{}, err
		}
	}

	appPod := appPod(instance)
	if err := ensureSame(r, instance, logger, appPod, &corev1.Pod{}, comparePods, checkPod); err != nil {
		return reconcile.Result{}, err
	}
	appService := appService(instance)
	if err := ensureSame(r, instance, logger, appService, &corev1.Service{}, compareService, nocheck); err != nil {
		return reconcile.Result{}, err
	}
	appRoute, err := appRoute(instance, r)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err := ensureSame(r, instance, logger, appRoute, &routev1.Route{}, compareRoute, checkRoute); err != nil {
		return reconcile.Result{}, err
	}

	if instance.Spec.Report.Enabled == nil || *instance.Spec.Report.Enabled {
		reportPod := reportPod(instance)
		if err := ensureSame(r, instance, logger, reportPod, &corev1.Pod{}, comparePods, checkPod); err != nil {
			return reconcile.Result{}, err
		}
		reportService := reportService(instance)
		if err := ensureSame(r, instance, logger, reportService, &corev1.Service{}, compareService, nocheck); err != nil {
			return reconcile.Result{}, err
		}
		reportRoute, err := reportRoute(instance, r)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := ensureSame(r, instance, logger, reportRoute, &routev1.Route{}, compareRoute, checkRoute); err != nil {
			return reconcile.Result{}, err
		}
	}

	uploadConfig := uploadConfig(instance)
	if err := ensureSame(r, instance, logger, uploadConfig, &corev1.ConfigMap{}, nocompare, nocheck); err != nil {
		return reconcile.Result{}, err
	}

	r.client.Status().Update(context.TODO(), instance)

	return reconcile.Result{}, nil
}

type resource interface {
	metav1.Object
	runtime.Object
}

func ensureSame(r *ReconcileHorreum, instance *hyperfoilv1alpha1.Horreum, logger logr.Logger,
	object resource, out runtime.Object,
	compare compareFunc, check checkFunc) error {
	// Set Hyperfoil instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, object, r.scheme); err != nil {
		return err
	}

	kind := reflect.TypeOf(object).Elem().Name()
	// Check if this Pod already exists
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, out)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new "+kind, kind+".Namespace", object.GetNamespace(), kind+".Name", object.GetName())
		err = r.client.Create(context.TODO(), object)
		if err != nil {
			updateStatus(r, instance, "Error", "Cannot create "+kind+" "+object.GetName())
			return err
		}
		setStatus(r, instance, "Pending", "Creating "+kind+" "+object.GetName())
	} else if err != nil {
		updateStatus(r, instance, "Error", "Cannot find "+kind+" "+object.GetName())
		return err
	} else if compare(object, out, logger) {
		logger.Info(kind + " " + object.GetName() + " already exists and matches.")
		if ok, status, reason := check(out); !ok {
			setStatus(r, instance, status, kind+" "+object.GetName()+" "+reason)
		}
	} else {
		logger.Info(kind + " " + object.GetName() + " already exists but does not match. Deleting existing object.")
		if err = r.client.Delete(context.TODO(), out); err != nil {
			logger.Error(err, "Cannot delete "+kind+" "+object.GetName())
			updateStatus(r, instance, "Error", "Cannot delete "+kind+" "+object.GetName())
			return err
		}
		logger.Info("Creating a new " + kind)
		if err = r.client.Create(context.TODO(), object); err != nil {
			updateStatus(r, instance, "Error", "Cannot create "+kind+" "+object.GetName())
			return err
		}
		setStatus(r, instance, "Pending", "Creating "+kind+" "+object.GetName())
	}
	return nil
}

func ensureDeleted(r *ReconcileHorreum, instance *hyperfoilv1alpha1.Horreum, object resource, out runtime.Object) error {
	kind := reflect.TypeOf(object).Elem().Name()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, out)
	if err != nil && errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		updateStatus(r, instance, "Error", "Cannot find "+kind+" "+object.GetName())
		return err
	} else {
		if err = r.client.Delete(context.TODO(), out); err != nil {
			updateStatus(r, instance, "Error", "Cannot delete "+kind+" "+object.GetName())
			return err
		}
	}
	return nil
}

func setStatus(r *ReconcileHorreum, instance *hyperfoilv1alpha1.Horreum, status string, reason string) {
	if instance.Status.Status == "Error" && status == "Pending" {
		return
	}
	instance.Status.Status = status
	instance.Status.Reason = reason
	instance.Status.LastUpdate = metav1.Now()
}

func updateStatus(r *ReconcileHorreum, instance *hyperfoilv1alpha1.Horreum, status string, reason string) {
	setStatus(r, instance, status, reason)
	r.client.Status().Update(context.TODO(), instance)
}

func comparePods(i1 interface{}, i2 interface{}, logger logr.Logger) bool {
	p1, ok1 := i1.(*corev1.Pod)
	p2, ok2 := i2.(*corev1.Pod)
	if !ok1 || !ok2 {
		logger.Info("Cannot cast to Pods: " + fmt.Sprintf("%v | %v", i1, i2))
		return false
	}
	if !compareContainerLists(p1.Spec.Containers, p2.Spec.Containers, logger) {
		return false
	}
	if !compareContainerLists(p1.Spec.InitContainers, p2.Spec.InitContainers, logger) {
		return false
	}
	return true
}

func compareContainerLists(cs1, cs2 []corev1.Container, logger logr.Logger) bool {
	if len(cs1) != len(cs2) {
		logger.Info("Containers don't match.")
		return false
	}
	for i := range cs1 {
		c1 := &cs1[i]
		c2 := &cs2[i]
		if !compareContainers(c1, c2, logger) {
			logger.Info("Containers don't match: " + fmt.Sprintf("%v | %v", c1, c2))
			return false
		}
	}
	return true
}

func compareContainers(c1 *corev1.Container, c2 *corev1.Container, logger logr.Logger) bool {
	if c1.Name != c2.Name {
		logger.Info("Names don't match: " + c1.Name + " | " + c2.Name)
		return false
	}
	if c1.Image != c2.Image {
		logger.Info("Images don't match: " + c1.Image + " | " + c2.Image)
		return false
	}
	if !reflect.DeepEqual(c1.Command, c2.Command) {
		logger.Info("Commands don't match: " + fmt.Sprintf("%v | %v", c1.Command, c2.Command))
		return false
	}
	if len(c1.Env) != len(c2.Env) {
		logger.Info("Envs don't match")
		return false
	}
	// TODO compare envs
	return true
}

func uploadConfig(cr *hyperfoilv1alpha1.Horreum) *corev1.ConfigMap {
	keycloakURL := `http://` + cr.Name + "-keycloak." + cr.Namespace + `.svc`
	if cr.Spec.Keycloak.External {
		keycloakURL = url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io")
	}
	horreumURL := `http://` + cr.Name + "." + cr.Namespace + `.svc`
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-hyperfoil-upload",
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"50-upload-to-horreum": `
			#!/bin/bash

			TOKEN=$(curl -s -X POST	` + keycloakURL + `/auth/realms/hyperfoil/protocol/openid-connect/token ` +
				` -H 'content-type: application/x-www-form-urlencoded' ` +
				` -d	'username='$HORREUM_USER'&password='$HORREUM_PASSWORD'&grant_type=password&client_id='$HORREUM_CLIENT_ID ` +
				` | jq -r .access_token)

			curl -s	'` + horreumURL + `/api/run/data?owner='$HORREUM_GROUP'&access=PUBLIC&test=$.info.benchmark&start=$.info.startTime&stop=$.info.terminateTime'` +
				` -X POST -H 'content-type: application/json' -d @$RUN_DIR/all.json -H 'Authorization: Bearer '$TOKEN
			`,
		},
	}
}

func checkSecret(keys ...string) checkFunc {
	return func(obj interface{}) (bool, string, string) {
		secret, ok := obj.(*corev1.Secret)
		if !ok {
			return false, "Error", " is not a secret"
		}
		for _, key := range keys {
			if _, ok := secret.Data[key]; !ok {
				return false, "Error", " missing data " + key
			}
		}
		return true, "", ""
	}
}

func checkPod(i interface{}) (bool, string, string) {
	pod, ok := i.(*corev1.Pod)
	if !ok {
		return false, "Error", " is not a pod"
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
				return false, "Error", " cannot pull container image"
			}
		} else if cs.State.Terminated != nil {
			return false, "Pending", " has terminated container"
		}
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true, "", ""
		}
	}
	return false, "Pending", " is not ready"
}

func compareService(i1, i2 interface{}, logger logr.Logger) bool {
	s1, ok1 := i1.(*corev1.Service)
	s2, ok2 := i1.(*corev1.Service)
	if !ok1 || !ok2 {
		logger.Info("Cannot cast to Secrets: " + fmt.Sprintf("%v | %v", i1, i2))
		return false
	}
	if len(s1.Spec.Ports) != len(s2.Spec.Ports) {
		logger.Info("Number of ports does not match: " + fmt.Sprintf("%v | %v", s1, s2))
		return false
	}
	for i, p1 := range s1.Spec.Ports {
		p2 := s2.Spec.Ports[i]
		if p1.Port != p2.Port {
			logger.Info("Ports don't match: " + fmt.Sprintf("%v | %v", s1, s2))
			return false
		}
	}
	return true
}

func compareRoute(i1, i2 interface{}, logger logr.Logger) bool {
	r1, ok1 := i1.(*routev1.Route)
	r2, ok2 := i2.(*routev1.Route)
	if !ok1 || !ok2 {
		logger.Info("Cannot cast to Routes: " + fmt.Sprintf("%v | %v", i1, i2))
		return false
	}
	if r1.Spec.Host == "" {
		return r1.Spec.Subdomain == r2.Spec.Subdomain
	}
	if r1.Spec.Host != r2.Spec.Host {
		return false
	}
	if !reflect.DeepEqual(r1.Spec.TLS, r2.Spec.TLS) {
		logger.Info("TLS configuration does not match: " + fmt.Sprintf("%v | %v", r1.Spec.TLS, r2.Spec.TLS))
		return false
	}
	return true
}

func checkRoute(i interface{}) (bool, string, string) {
	route, ok := i.(*routev1.Route)
	if !ok {
		return false, "Error", " is not a route"
	}
	for _, ri := range route.Status.Ingress {
		for _, c := range ri.Conditions {
			if c.Type == routev1.RouteAdmitted {
				if c.Status == corev1.ConditionTrue {
					return true, "", ""
				}
				return false, "Error", " was not admitted"
			}
		}
	}
	return false, "Pending", " is in unknown state"
}
