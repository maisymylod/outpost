// Package v1 contains the outpost.dev/v1 Workload API types reconciled by the
// operator. The group/version here must match the rendered CRD and CR.
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group and version for the Workload kind.
var GroupVersion = schema.GroupVersion{Group: "outpost.dev", Version: "v1"}

// SchemeBuilder registers the Workload types into a runtime scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds the Workload types to the given scheme.
var AddToScheme = SchemeBuilder.AddToScheme

func init() {
	SchemeBuilder.Register(&Workload{}, &WorkloadList{})
}
