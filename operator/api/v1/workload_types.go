package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadSpec describes the desired state of a GPU workload. It mirrors the
// outpost spec's workload block so one declarative source drives every target.
type WorkloadSpec struct {
	// Image is the container image to run.
	Image string `json:"image"`
	// Replicas is the number of pods.
	Replicas int32 `json:"replicas"`
	// GPU describes per-replica accelerator requirements.
	GPU GPU `json:"gpu"`
	// Resources are the non-GPU container resources.
	Resources Resources `json:"resources"`
	// Env is a set of environment variables for the container.
	Env map[string]string `json:"env,omitempty"`
	// Ports are the container ports to expose via the Service.
	Ports []Port `json:"ports,omitempty"`
	// Command overrides the image entrypoint.
	Command []string `json:"command,omitempty"`
	// Args overrides the image arguments.
	Args []string `json:"args,omitempty"`
}

// GPU captures per-replica accelerator requirements and interconnect.
type GPU struct {
	Count        int32        `json:"count"`
	Class        string       `json:"class"`
	Interconnect Interconnect `json:"interconnect"`
}

// Interconnect toggles the multi-GPU fabric features.
type Interconnect struct {
	NCCL       bool `json:"nccl"`
	InfiniBand bool `json:"infiniband"`
}

// Resources are the non-GPU container resource requests/limits.
type Resources struct {
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	HugePages string `json:"hugepages,omitempty"`
}

// Port is a named container port.
type Port struct {
	Name string `json:"name"`
	Port int32  `json:"port"`
}

// WorkloadStatus is the observed state of a Workload.
type WorkloadStatus struct {
	// ObservedReplicas is the replica count last applied to the Deployment.
	ObservedReplicas int32 `json:"observedReplicas,omitempty"`
	// Ready reports whether the managed Deployment and Service exist.
	Ready bool `json:"ready,omitempty"`
}

// Workload is the Schema for the workloads API.
type Workload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadSpec   `json:"spec,omitempty"`
	Status WorkloadStatus `json:"status,omitempty"`
}

// WorkloadList contains a list of Workload.
type WorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workload `json:"items"`
}
