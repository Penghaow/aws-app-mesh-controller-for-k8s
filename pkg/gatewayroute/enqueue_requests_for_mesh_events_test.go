// +build preview

package gatewayroute

import (
	"context"
	appmesh "github.com/aws/aws-app-mesh-controller-for-k8s/apis/appmesh/v1beta2"
	"github.com/aws/aws-app-mesh-controller-for-k8s/pkg/k8s"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

func Test_enqueueRequestsForMeshEvents_Update(t *testing.T) {
	gr1 := &appmesh.GatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "gr-1",
		},
		Spec: appmesh.GatewayRouteSpec{
			MeshRef: &appmesh.MeshReference{
				Name: "my-mesh",
				UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
			},
		},
	}
	gr2 := &appmesh.GatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "gr-2",
		},
		Spec: appmesh.GatewayRouteSpec{
			MeshRef: &appmesh.MeshReference{
				Name: "my-mesh",
				UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
			},
		},
	}

	type env struct {
		gatewayRoutes []*appmesh.GatewayRoute
	}
	type args struct {
		e event.UpdateEvent
	}
	tests := []struct {
		name         string
		env          env
		args         args
		wantRequests []reconcile.Request
	}{
		{
			name: "meshActive status changed",
			env: env{
				gatewayRoutes: []*appmesh.GatewayRoute{gr1, gr2},
			},
			args: args{
				e: event.UpdateEvent{
					ObjectOld: &appmesh.Mesh{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-mesh",
							UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
						},
					},
					ObjectNew: &appmesh.Mesh{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-mesh",
							UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
						},
						Status: appmesh.MeshStatus{
							Conditions: []appmesh.MeshCondition{
								{
									Type:   appmesh.MeshActive,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
			},
			wantRequests: nil,
		},
		{
			name: "meshActive status changed",
			env: env{
				gatewayRoutes: []*appmesh.GatewayRoute{gr1, gr2},
			},
			args: args{
				e: event.UpdateEvent{
					ObjectOld: &appmesh.Mesh{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-mesh",
							UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
						},
					},
					ObjectNew: &appmesh.Mesh{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-mesh",
							UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
						},
						Status: appmesh.MeshStatus{
							Conditions: []appmesh.MeshCondition{
								{
									Type:   appmesh.MeshActive,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			wantRequests: []reconcile.Request{
				{
					NamespacedName: k8s.NamespacedName(gr1),
				},
				{
					NamespacedName: k8s.NamespacedName(gr2),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			appmesh.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			h := &enqueueRequestsForMeshEvents{
				k8sClient: k8sClient,
				log:       &log.NullLogger{},
			}

			for _, gr := range tt.env.gatewayRoutes {
				err := k8sClient.Create(ctx, gr.DeepCopy())
				assert.NoError(t, err)
			}

			h.Update(tt.args.e, queue)
			var gotRequests []reconcile.Request
			queueLen := queue.Len()
			for i := 0; i < queueLen; i++ {
				item, _ := queue.Get()
				gotRequests = append(gotRequests, item.(reconcile.Request))
			}

			opt := cmpopts.SortSlices(compareReconcileRequest)
			assert.True(t, cmp.Equal(tt.wantRequests, gotRequests, opt), "diff: %v", cmp.Diff(tt.wantRequests, gotRequests, opt))

		})
	}
}

func Test_enqueueRequestsForMeshEvents_enqueueGatewayRoutesForMesh(t *testing.T) {
	ms := &appmesh.Mesh{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-mesh",
			UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
		},
		Status: appmesh.MeshStatus{
			Conditions: []appmesh.MeshCondition{
				{
					Type:   appmesh.MeshActive,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	grWithoutMeshRef := &appmesh.GatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "gr-without-mesh-ref",
		},
		Spec: appmesh.GatewayRouteSpec{},
	}
	grWithNonMatchingMeshRef := &appmesh.GatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "gr-with-non-matching-mesh-ref",
		},
		Spec: appmesh.GatewayRouteSpec{
			MeshRef: &appmesh.MeshReference{
				Name: "my-mesh",
				UID:  "0d65db83-1b4c-40aa-90ba-57064dd73c98",
			},
		},
	}
	grWithMatchingMeshRef_1 := &appmesh.GatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "gr-with-non-matching-mesh-ref-1",
		},
		Spec: appmesh.GatewayRouteSpec{
			MeshRef: &appmesh.MeshReference{
				Name: "my-mesh",
				UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
			},
		},
	}
	grWithMatchingMeshRef_2 := &appmesh.GatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "gr-with-non-matching-mesh-ref-2",
		},
		Spec: appmesh.GatewayRouteSpec{
			MeshRef: &appmesh.MeshReference{
				Name: "my-mesh",
				UID:  "a385048d-aba8-4235-9a11-4173764c8ab7",
			},
		},
	}

	type env struct {
		gatewayRoutes []*appmesh.GatewayRoute
	}
	type args struct {
		ms *appmesh.Mesh
	}
	tests := []struct {
		name         string
		env          env
		args         args
		wantRequests []reconcile.Request
	}{
		{
			name: "gr without meshRef shouldn't be enqueued",
			env: env{
				gatewayRoutes: []*appmesh.GatewayRoute{
					grWithoutMeshRef,
				},
			},
			args: args{
				ms: ms,
			},
			wantRequests: nil,
		},
		{
			name: "gr with non-matching meshRef shouldn't be enqueued",
			env: env{
				gatewayRoutes: []*appmesh.GatewayRoute{
					grWithNonMatchingMeshRef,
				},
			},
			args: args{
				ms: ms,
			},
			wantRequests: nil,
		},
		{
			name: "gr with matching meshRef should be enqueued",
			env: env{
				gatewayRoutes: []*appmesh.GatewayRoute{
					grWithMatchingMeshRef_1,
				},
			},
			args: args{
				ms: ms,
			},
			wantRequests: []reconcile.Request{
				{
					NamespacedName: k8s.NamespacedName(grWithMatchingMeshRef_1),
				},
			},
		},
		{
			name: "multiple gr should enqueue correctly",
			env: env{
				gatewayRoutes: []*appmesh.GatewayRoute{
					grWithoutMeshRef,
					grWithNonMatchingMeshRef,
					grWithMatchingMeshRef_1,
					grWithMatchingMeshRef_2,
				},
			},
			args: args{
				ms: ms,
			},
			wantRequests: []reconcile.Request{
				{
					NamespacedName: k8s.NamespacedName(grWithMatchingMeshRef_1),
				},
				{
					NamespacedName: k8s.NamespacedName(grWithMatchingMeshRef_2),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			appmesh.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			h := &enqueueRequestsForMeshEvents{
				k8sClient: k8sClient,
				log:       &log.NullLogger{},
			}

			for _, gr := range tt.env.gatewayRoutes {
				err := k8sClient.Create(ctx, gr.DeepCopy())
				assert.NoError(t, err)
			}

			h.enqueueGatewayRoutesForMesh(ctx, queue, tt.args.ms)
			var gotRequests []reconcile.Request
			queueLen := queue.Len()
			for i := 0; i < queueLen; i++ {
				item, _ := queue.Get()
				gotRequests = append(gotRequests, item.(reconcile.Request))
			}

			opt := cmpopts.SortSlices(compareReconcileRequest)
			assert.True(t, cmp.Equal(tt.wantRequests, gotRequests, opt), "diff: %v", cmp.Diff(tt.wantRequests, gotRequests, opt))
		})
	}
}

func compareReconcileRequest(a reconcile.Request, b reconcile.Request) bool {
	return a.String() < b.String()
}
