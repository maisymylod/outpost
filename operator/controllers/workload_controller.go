// Package controllers contains the Workload reconciler. It drives a Workload CR
// toward the desired Deployment and Service produced by the workload package.
package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	outpostv1 "github.com/maisymylod/outpost/operator/api/v1"
	"github.com/maisymylod/outpost/operator/internal/workload"
)

// WorkloadReconciler reconciles a Workload object into a Deployment + Service.
type WorkloadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=outpost.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=outpost.dev,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile brings the cluster in line with one Workload's desired state.
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var wl outpostv1.Workload
	if err := r.Get(ctx, req.NamespacedName, &wl); err != nil {
		// Ignore not-found: the CR was deleted and owned objects are GC'd.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.applyDeployment(ctx, &wl); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyService(ctx, &wl); err != nil {
		return ctrl.Result{}, err
	}

	wl.Status.ObservedReplicas = wl.Spec.Replicas
	wl.Status.Ready = true
	if err := r.Status().Update(ctx, &wl); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// applyDeployment creates or updates the managed Deployment, owned by the CR.
func (r *WorkloadReconciler) applyDeployment(ctx context.Context, wl *outpostv1.Workload) error {
	dep := &appsv1.Deployment{}
	dep.Name = wl.Name
	dep.Namespace = wl.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		desired, err := workload.BuildDeployment(wl)
		if err != nil {
			return err
		}
		dep.Labels = desired.Labels
		dep.Spec = desired.Spec
		return controllerutil.SetControllerReference(wl, dep, r.Scheme)
	})
	return err
}

// applyService creates or updates the managed Service, owned by the CR.
func (r *WorkloadReconciler) applyService(ctx context.Context, wl *outpostv1.Workload) error {
	svc := &corev1.Service{}
	svc.Name = wl.Name
	svc.Namespace = wl.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		desired := workload.BuildService(wl)
		svc.Labels = desired.Labels
		// Preserve the cluster-assigned ClusterIP across updates.
		clusterIP := svc.Spec.ClusterIP
		svc.Spec = desired.Spec
		svc.Spec.ClusterIP = clusterIP
		return controllerutil.SetControllerReference(wl, svc, r.Scheme)
	})
	return err
}

// SetupWithManager registers the reconciler and the objects it owns.
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&outpostv1.Workload{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
