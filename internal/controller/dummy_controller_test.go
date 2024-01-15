package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	dummyv1 "github.com/sonalys/dummy-k8s-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_DummyReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, dummyv1.AddToScheme(scheme))

	t.Run("success: no object", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme)

		controller := DummyReconciler{
			Client:          fakeClient.Build(),
			Scheme:          runtime.NewScheme(),
			LoggedDummies:   make(map[string]struct{}),
			LoggedDummyLock: &sync.RWMutex{},
		}

		ctx := context.Background()
		res, err := controller.Reconcile(ctx, reconcile.Request{})
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, res)
		// require.Equal(t, obj.Spec.Message, obj.Status.SpecEcho)
	})

	t.Run("success: no pod", func(t *testing.T) {
		obj := &dummyv1.Dummy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy1",
			},
			Spec: dummyv1.DummySpec{Message: "I'm just a dummy"},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(obj)

		controller := DummyReconciler{
			Client:          fakeClient.Build(),
			Scheme:          runtime.NewScheme(),
			LoggedDummies:   make(map[string]struct{}),
			LoggedDummyLock: &sync.RWMutex{},
		}

		ctx := context.Background()
		res, err := controller.Reconcile(ctx, reconcile.Request{})
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, res)
		// require.Equal(t, obj.Spec.Message, obj.Status.SpecEcho)
	})

	t.Run("success: with pod", func(t *testing.T) {
		obj := &dummyv1.Dummy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy1",
			},
			Spec: dummyv1.DummySpec{Message: "I'm just a dummy"},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(obj).
			WithObjects(&corev1.Pod{
				Spec: createDummyPodSpec(obj),
				ObjectMeta: metav1.ObjectMeta{
					Name:      obj.Name,
					Namespace: obj.Namespace,
				},
			})

		controller := DummyReconciler{
			Client:          fakeClient.Build(),
			Scheme:          runtime.NewScheme(),
			LoggedDummies:   make(map[string]struct{}),
			LoggedDummyLock: &sync.RWMutex{},
		}

		ctx := context.Background()
		res, err := controller.Reconcile(ctx, reconcile.Request{})
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, res)
		// require.Equal(t, obj.Spec.Message, obj.Status.SpecEcho)
	})
}

func Test_updateDummySpecStatus(t *testing.T) {
	t.Run("success: updated", func(t *testing.T) {
		dummy := &dummyv1.Dummy{
			Spec: dummyv1.DummySpec{
				Message: "message",
			},
		}
		got := updateDummySpecStatus(dummy)
		require.True(t, got)
	})
	t.Run("success: unchanged", func(t *testing.T) {
		dummy := &dummyv1.Dummy{
			Spec: dummyv1.DummySpec{
				Message: "message",
			},
			Status: dummyv1.DummyStatus{
				SpecEcho: "message",
			},
		}
		got := updateDummySpecStatus(dummy)
		require.False(t, got)
	})
}

func Test_updateDummyPodStatus(t *testing.T) {
	ctx := context.Background()
	t.Run("success: from unknown to pending", func(t *testing.T) {
		dummy := &dummyv1.Dummy{}
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}
		got := updateDummyPodStatus(ctx, dummy, pod)
		require.True(t, got)
		require.Equal(t, dummyv1.DummyStatusTypePending, dummy.Status.PodStatus)
	})
	t.Run("success: from pending to running", func(t *testing.T) {
		dummy := &dummyv1.Dummy{
			Status: dummyv1.DummyStatus{
				PodStatus: dummyv1.DummyStatusTypePending,
			},
		}
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		got := updateDummyPodStatus(ctx, dummy, pod)
		require.True(t, got)
		require.Equal(t, dummyv1.DummyStatusTypeRunning, dummy.Status.PodStatus)
	})
	t.Run("success: unchanged", func(t *testing.T) {
		dummy := &dummyv1.Dummy{
			Status: dummyv1.DummyStatus{
				PodStatus: dummyv1.DummyStatusTypeRunning,
			},
		}
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		got := updateDummyPodStatus(ctx, dummy, pod)
		require.False(t, got)
	})
}

func Test_registerDummy(t *testing.T) {
	controller := DummyReconciler{
		LoggedDummies:   make(map[string]struct{}),
		LoggedDummyLock: &sync.RWMutex{},
	}
	require.True(t, controller.registerDummy("123"), "should register")
	require.False(t, controller.registerDummy("123"), "should not register")
}
