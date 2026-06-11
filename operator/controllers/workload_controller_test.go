package controllers

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	outpostv1 "github.com/maisymylod/outpost/operator/api/v1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgo scheme: %v", err)
	}
	if err := outpostv1.AddToScheme(s); err != nil {
		t.Fatalf("outpost scheme: %v", err)
	}
	return s
}

func sampleWorkload() *outpostv1.Workload {
	return &outpostv1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "llm-inference", Namespace: "inference"},
		Spec: outpostv1.WorkloadSpec{
			Image:    "ghcr.io/example/vllm-server:0.6.3",
			Replicas: 2,
			GPU: outpostv1.GPU{
				Count:        4,
				Class:        "nvidia-a100-80gb",
				Interconnect: outpostv1.Interconnect{NCCL: true, InfiniBand: true},
			},
			Resources: outpostv1.Resources{CPU: "16", Memory: "128Gi", HugePages: "2Gi"},
			Env:       map[string]string{"MODEL_ID": "demo"},
			Ports:     []outpostv1.Port{{Name: "http", Port: 8000}},
		},
	}
}

func TestReconcileCreatesDeploymentAndService(t *testing.T) {
	scheme := testScheme(t)
	wl := sampleWorkload()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(wl).
		WithStatusSubresource(wl).
		Build()

	r := &WorkloadReconciler{Client: c, Scheme: scheme}
	key := types.NamespacedName{Name: "llm-inference", Namespace: "inference"}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Deployment created with the right replica count and GPU wiring.
	var dep appsv1.Deployment
	if err := c.Get(context.Background(), key, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 2 {
		t.Errorf("replicas = %v, want 2", dep.Spec.Replicas)
	}
	if len(dep.OwnerReferences) != 1 || dep.OwnerReferences[0].Kind != "Workload" {
		t.Errorf("deployment owner refs = %+v, want one Workload owner", dep.OwnerReferences)
	}

	cntr := dep.Spec.Template.Spec.Containers[0]
	gpu := cntr.Resources.Limits["nvidia.com/gpu"]
	if gpu.Value() != 4 {
		t.Errorf("nvidia.com/gpu = %d, want 4", gpu.Value())
	}
	if _, ok := cntr.Resources.Limits["rdma/ib"]; !ok {
		t.Error("expected rdma/ib resource for infiniband workload")
	}
	if _, ok := cntr.Resources.Limits["hugepages-2Mi"]; !ok {
		t.Error("expected hugepages limit for infiniband workload")
	}

	if !hasEnv(cntr.Env, "NCCL_IB_HCA", "mlx5") {
		t.Error("expected NCCL_IB_HCA=mlx5 env")
	}
	if !hasEnv(cntr.Env, "NCCL_DEBUG", "INFO") {
		t.Error("expected NCCL_DEBUG=INFO env")
	}
	if !hasEnv(cntr.Env, "MODEL_ID", "demo") {
		t.Error("expected user env MODEL_ID=demo")
	}
	// User env must precede derived env for deterministic ordering.
	if cntr.Env[0].Name != "MODEL_ID" {
		t.Errorf("first env = %q, want MODEL_ID", cntr.Env[0].Name)
	}

	if cntr.SecurityContext == nil || len(cntr.SecurityContext.Capabilities.Add) == 0 {
		t.Error("expected IPC_LOCK capability for infiniband workload")
	}

	if len(dep.Spec.Template.Spec.Tolerations) == 0 {
		t.Error("expected a GPU toleration")
	}
	if dep.Spec.Template.Spec.Affinity == nil || dep.Spec.Template.Spec.Affinity.NodeAffinity == nil {
		t.Error("expected GPU node affinity")
	}

	// Service created with the workload port.
	var svc corev1.Service
	if err := c.Get(context.Background(), key, &svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 8000 {
		t.Errorf("service ports = %+v, want one port 8000", svc.Spec.Ports)
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	scheme := testScheme(t)
	wl := sampleWorkload()
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).WithStatusSubresource(wl).Build()
	r := &WorkloadReconciler{Client: c, Scheme: scheme}
	key := types.NamespacedName{Name: "llm-inference", Namespace: "inference"}

	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
			t.Fatalf("Reconcile pass %d: %v", i, err)
		}
	}

	var deps appsv1.DeploymentList
	if err := c.List(context.Background(), &deps); err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deps.Items) != 1 {
		t.Errorf("deployment count = %d, want 1 after repeated reconciles", len(deps.Items))
	}
}

func TestReconcileMissingWorkloadIsNoop(t *testing.T) {
	scheme := testScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &WorkloadReconciler{Client: c, Scheme: scheme}
	key := types.NamespacedName{Name: "ghost", Namespace: "nowhere"}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("Reconcile on missing CR should be a noop, got %v", err)
	}
}

func hasEnv(env []corev1.EnvVar, name, val string) bool {
	for _, e := range env {
		if e.Name == name {
			return e.Value == val
		}
	}
	return false
}
