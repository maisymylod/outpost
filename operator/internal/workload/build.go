// Package workload translates a Workload spec into the Kubernetes objects the
// operator manages: a Deployment (with full GPU/NCCL/InfiniBand wiring) and a
// Service. Keeping the translation here lets it be unit-tested without a
// cluster and reused verbatim by the reconciler.
package workload

import (
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	outpostv1 "github.com/maisymylod/outpost/operator/api/v1"
)

// Resource names for accelerator and fabric devices exposed by their
// respective Kubernetes device plugins.
const (
	resourceGPU       = "nvidia.com/gpu"
	resourceRDMA      = "rdma/ib"
	resourceHugePages = "hugepages-2Mi"

	// gpuProductLabel is published by NVIDIA GPU Feature Discovery and lets us
	// pin pods to nodes carrying the requested GPU class.
	gpuProductLabel = "nvidia.com/gpu.product"
	// gpuTaintKey is the standard taint the device plugin places on GPU nodes.
	gpuTaintKey = "nvidia.com/gpu"

	containerName = "workload"
	hugePageMount = "/dev/hugepages"
	shmMount      = "/dev/shm"
)

// Labels returns the deterministic label set shared by all managed objects.
func Labels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/managed-by": "outpost-operator",
	}
}

// BuildDeployment renders the Deployment for a Workload, wiring GPU resource
// requests, NVIDIA device-plugin tolerations/affinity, NCCL and InfiniBand
// environment, hugepages, and shared memory as the spec requires.
func BuildDeployment(wl *outpostv1.Workload) (*appsv1.Deployment, error) {
	limits, err := resourceLimits(&wl.Spec)
	if err != nil {
		return nil, err
	}

	labels := Labels(wl.Name)
	replicas := wl.Spec.Replicas

	container := corev1.Container{
		Name:    containerName,
		Image:   wl.Spec.Image,
		Command: wl.Spec.Command,
		Args:    wl.Spec.Args,
		Env:     buildEnv(&wl.Spec),
		Ports:   buildContainerPorts(wl.Spec.Ports),
		Resources: corev1.ResourceRequirements{
			Limits:   limits,
			Requests: limits,
		},
	}

	volumes, mounts := buildVolumes(&wl.Spec)
	container.VolumeMounts = mounts

	podSpec := corev1.PodSpec{
		Containers:  []corev1.Container{container},
		Volumes:     volumes,
		Tolerations: gpuTolerations(),
		Affinity:    gpuAffinity(wl.Spec.GPU.Class),
	}
	if wl.Spec.GPU.Interconnect.InfiniBand {
		// RDMA verbs need locked memory; grant IPC_LOCK to the container.
		podSpec.Containers[0].SecurityContext = &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"IPC_LOCK"}},
		}
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wl.Name,
			Namespace: wl.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}, nil
}

// BuildService renders the ClusterIP Service exposing the workload ports.
func BuildService(wl *outpostv1.Workload) *corev1.Service {
	labels := Labels(wl.Name)
	ports := make([]corev1.ServicePort, 0, len(wl.Spec.Ports))
	for _, p := range wl.Spec.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: intstr.FromInt32(p.Port),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wl.Name,
			Namespace: wl.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    ports,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

// resourceLimits builds the container resource list: CPU, memory, GPUs, and
// (when InfiniBand is on) an RDMA device plus hugepages.
func resourceLimits(s *outpostv1.WorkloadSpec) (corev1.ResourceList, error) {
	cpu, err := resource.ParseQuantity(s.Resources.CPU)
	if err != nil {
		return nil, fmt.Errorf("resources.cpu %q: %w", s.Resources.CPU, err)
	}
	mem, err := resource.ParseQuantity(s.Resources.Memory)
	if err != nil {
		return nil, fmt.Errorf("resources.memory %q: %w", s.Resources.Memory, err)
	}

	limits := corev1.ResourceList{
		corev1.ResourceCPU:    cpu,
		corev1.ResourceMemory: mem,
		resourceGPU:           *resource.NewQuantity(int64(s.GPU.Count), resource.DecimalSI),
	}

	if s.GPU.Interconnect.InfiniBand {
		limits[resourceRDMA] = *resource.NewQuantity(1, resource.DecimalSI)
		if s.Resources.HugePages != "" {
			hp, err := resource.ParseQuantity(s.Resources.HugePages)
			if err != nil {
				return nil, fmt.Errorf("resources.hugepages %q: %w", s.Resources.HugePages, err)
			}
			limits[resourceHugePages] = hp
		}
	}
	return limits, nil
}

// buildEnv returns user env (sorted for determinism) followed by the derived
// NCCL and InfiniBand variables in a fixed order.
func buildEnv(s *outpostv1.WorkloadSpec) []corev1.EnvVar {
	var env []corev1.EnvVar

	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, corev1.EnvVar{Name: k, Value: s.Env[k]})
	}

	if s.GPU.Interconnect.NCCL {
		env = append(env,
			corev1.EnvVar{Name: "NCCL_DEBUG", Value: "INFO"},
			corev1.EnvVar{Name: "NCCL_P2P_LEVEL", Value: "NVL"},
			corev1.EnvVar{Name: "NCCL_SOCKET_IFNAME", Value: "^lo,docker0"},
		)
	}
	if s.GPU.Interconnect.InfiniBand {
		env = append(env,
			corev1.EnvVar{Name: "NCCL_IB_DISABLE", Value: "0"},
			corev1.EnvVar{Name: "NCCL_IB_HCA", Value: "mlx5"},
			corev1.EnvVar{Name: "NCCL_NET_GDR_LEVEL", Value: "5"},
		)
	}
	return env
}

// buildContainerPorts maps spec ports to container ports.
func buildContainerPorts(ports []outpostv1.Port) []corev1.ContainerPort {
	out := make([]corev1.ContainerPort, 0, len(ports))
	for _, p := range ports {
		out = append(out, corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.Port,
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return out
}

// buildVolumes adds a large shared-memory volume for NCCL and a hugepages
// volume for InfiniBand/RDMA, with matching mounts.
func buildVolumes(s *outpostv1.WorkloadSpec) ([]corev1.Volume, []corev1.VolumeMount) {
	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount

	if s.GPU.Interconnect.NCCL {
		volumes = append(volumes, corev1.Volume{
			Name: "dshm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "dshm", MountPath: shmMount})
	}
	if s.GPU.Interconnect.InfiniBand {
		volumes = append(volumes, corev1.Volume{
			Name: "hugepages",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumHugePages},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "hugepages", MountPath: hugePageMount})
	}
	return volumes, mounts
}

// gpuTolerations lets pods schedule onto tainted GPU nodes.
func gpuTolerations() []corev1.Toleration {
	return []corev1.Toleration{{
		Key:      gpuTaintKey,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}}
}

// gpuAffinity pins pods to nodes advertising the requested GPU class.
func gpuAffinity(class string) *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      gpuProductLabel,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{class},
					}},
				}},
			},
		},
	}
}
