/*
Copyright 2024.

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

package controller

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pkg/errors"
	dummyv1 "github.com/sonalys/dummy-k8s-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DummyReconciler reconciles a Dummy object
type DummyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	LoggedDummies   map[string]struct{}
	LoggedDummyLock *sync.RWMutex
}

//+kubebuilder:rbac:groups=dummy.interview.com,resources=dummies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dummy.interview.com,resources=dummies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dummy.interview.com,resources=dummies/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *DummyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dummyList dummyv1.DummyList
	result := ctrl.Result{
		RequeueAfter: 5 * time.Second,
	}
	if err := r.List(ctx, &dummyList, client.InNamespace(req.Namespace)); err != nil {
		return result, client.IgnoreNotFound(err)
	}
	for i := range dummyList.Items {
		dummy := &dummyList.Items[i]
		r.logDummyObjects(ctx, dummy)
		pod, err := r.getOrCreatePodForDummy(ctx, req.NamespacedName, dummy)
		if err != nil {
			return result, errors.Wrapf(err, "retrieving dummy pod")
		}
		if err := r.updateDummy(ctx, dummy, pod); err != nil {
			return result, errors.Wrapf(err, "updating dummy object")
		}
	}
	return result, nil
}

// updateDummyPodStatus receives Dummy object and it's pod.
// It returns true if the status has been updated, otherwise false.
func updateDummyPodStatus(ctx context.Context, dummy *dummyv1.Dummy, pod *corev1.Pod) bool {
	logger := log.FromContext(ctx)
	// Update the status of the postgresql object based on the status of the Pod
	var newStatus dummyv1.DummyStatusType
	switch pod.Status.Phase {
	case corev1.PodPending:
		newStatus = dummyv1.DummyStatusTypePending
	case corev1.PodRunning:
		newStatus = dummyv1.DummyStatusTypeRunning
	default:
		newStatus = dummyv1.DummyStatusTypeFailed
	}
	if newStatus == dummy.Status.PodStatus {
		return false
	}
	logger.Info("Dummy pod status changed", "oldStatus", dummy.Status.PodStatus, "newStatus", newStatus)
	dummy.Status.PodStatus = newStatus
	return true
}

// updateDummySpecStatus updates Dummy's spec.status,
// returns true if the status has been updated, otherwise false.
func updateDummySpecStatus(dummy *dummyv1.Dummy) bool {
	if dummy.Status.SpecEcho != dummy.Spec.Message {
		dummy.Status.SpecEcho = dummy.Spec.Message
		return true
	}
	return false
}

// updateDummy will try to update Dummy's fields, if no change is required, it won't be updated.
func (r *DummyReconciler) updateDummy(ctx context.Context, dummy *dummyv1.Dummy, pod *corev1.Pod) error {
	if !updateDummyPodStatus(ctx, dummy, pod) && !updateDummySpecStatus(dummy) {
		return nil
	}
	if err := r.Status().Update(ctx, dummy); err != nil {
		return errors.Wrapf(err, "failed to update status")
	}
	return nil
}

// registerDummy will try to register a new dummy into the reconciler,
// if it's already registered, return false, otherwise true.
func (r *DummyReconciler) registerDummy(uid string) bool {
	r.LoggedDummyLock.RLock()
	if _, exists := r.LoggedDummies[uid]; exists {
		r.LoggedDummyLock.RUnlock()
		return false
	}
	r.LoggedDummyLock.RUnlock()
	r.LoggedDummyLock.Lock()
	r.LoggedDummies[uid] = struct{}{}
	r.LoggedDummyLock.Unlock()
	return true
}

// logDummyObjects logs new dummies that are registered into the reconciler.
func (r *DummyReconciler) logDummyObjects(ctx context.Context, dummy *dummyv1.Dummy) {
	if !r.registerDummy(string(dummy.UID)) {
		return
	}
	log.FromContext(ctx).
		Info("Detected new Dummy object", "namespace", dummy.Namespace, "name", dummy.Name, "spec.message", dummy.Spec.Message)
}

func createDummyPodSpec(dummy *dummyv1.Dummy) corev1.PodSpec {
	container := corev1.Container{
		Name:  dummy.Name,
		Image: "nginx",
	}
	return corev1.PodSpec{
		Containers: []corev1.Container{container},
	}
}

// getOrCreatePodForDummy will try to fetch any existing pods related to dummy, if none is found, creates a new one.
func (r *DummyReconciler) getOrCreatePodForDummy(ctx context.Context, ns types.NamespacedName, obj *dummyv1.Dummy) (*corev1.Pod, error) {
	logger := log.FromContext(ctx)
	var pod corev1.Pod
	// If no corresponding pod exists, create one.
	if err := r.Get(ctx, ns, &pod); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrapf(err, "getting pod failed")
		}
		// A notFound error means we should create a pod.
		pod = corev1.Pod{
			Spec: createDummyPodSpec(obj),
			ObjectMeta: metav1.ObjectMeta{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			},
		}
		logger.Info("Creating pod for dummy", "name", pod.Name)
		if err := r.Create(ctx, &pod); err != nil {
			return nil, errors.Wrapf(err, "creating pod failed")
		}
	}
	return &pod, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DummyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dummyv1.Dummy{}).
		Complete(r)
}
