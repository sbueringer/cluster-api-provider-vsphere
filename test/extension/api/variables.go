/*
Copyright 2023 The Kubernetes Authors.

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

package api

import (
	_ "embed"
	"encoding/json"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	//go:embed zz_generated.variables.json
	variableDefinitionsBytes []byte

	// VariableDefinitions contains the variable definitions of this Runtime Extension.
	// The VariableDefinitions will be parsed from variableDefinitionsBytes (zz_generated.variables.json)
	// and then returned in the DiscoverVariables hook.
	VariableDefinitions []clusterv1.ClusterClassVariable
)

func init() {
	if err := json.Unmarshal(variableDefinitionsBytes, &VariableDefinitions); err != nil {
		panic(errors.Wrap(err, "failed to parse variable definitions"))
	}
}

// Variables defines the schemas of the variables.
// Each top-level field will be exactly one variable.
// FIXME: These variables are just copied from core CAPI, we probably want to define others.
type Variables struct {
	// LBImageRepository is the image repository of the load balancer.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=kindest
	LBImageRepository string `json:"lbImageRepository"`

	// ImageRepository sets the container registry to pull images from. If empty, `registry.k8s.io` will be used by default.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=registry.k8s.io
	ImageRepository string `json:"imageRepository"`

	// KubeadmControlPlaneMaxSurge is the maximum number of control planes that can be scheduled above or under the desired number of control plane machines.
	// +kubebuilder:example="0"
	// +optional
	KubeadmControlPlaneMaxSurge string `json:"kubeadmControlPlaneMaxSurge,omitempty"`

	// ControlPlaneCertificateRotation configures cert rotation.
	// +kubebuilder:default={activate: true, daysBefore: 90}
	// +kubebuilder:example={activate: true, daysBefore: 90}
	// +optional
	ControlPlaneCertificateRotation *ControlPlaneCertificateRotation `json:"controlPlaneCertificateRotation,omitempty"`
}

// ControlPlaneCertificateRotation configures cert rotation.
type ControlPlaneCertificateRotation struct {
	// Activate activates cert rotation.
	// +kubebuilder:default=true
	// +optional
	Activate bool `json:"activate"`

	// DaysBefore configures how many days before expiry control plane machines are rotated.
	// +kubebuilder:default=90
	// +kubebuilder:validation:Minimum=7
	// +optional
	DaysBefore int32 `json:"daysBefore"`
}
