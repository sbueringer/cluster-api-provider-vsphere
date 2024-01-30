/*
Copyright 2022 The Kubernetes Authors.

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

// Package topologymutation contains the handlers for the topologymutation webhook.
//
// When implementing custom RuntimeExtension, it is only required to expose the required HandlerFunc with the
// signature defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
package topologymutation

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	vmwarev1 "sigs.k8s.io/cluster-api-provider-vsphere/apis/vmware/v1beta1"
	"sigs.k8s.io/cluster-api-provider-vsphere/test/extension/api"
	patchvariables "sigs.k8s.io/cluster-api-provider-vsphere/test/extension/third_party/cluster-api/controllers/topology/cluster/patches/variables"
	"sigs.k8s.io/cluster-api-provider-vsphere/test/extension/third_party/cluster-api/exp/runtime/topologymutation"
)

// ExtensionHandlers provides a common struct shared across the topology mutation hook handlers; this is convenient
// because it allows to easily provide a controller runtime client to the handler functions if necessary.
// NOTE: it is not mandatory to use a ExtensionHandlers struct in custom RuntimeExtension, what is important
// is to expose the required HandlerFunc with the signatures defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
type ExtensionHandlers struct {
	decoder runtime.Decoder
}

// NewExtensionHandlers returns a new ExtensionHandlers for the topology mutation hook handlers.
func NewExtensionHandlers(scheme *runtime.Scheme) *ExtensionHandlers {
	return &ExtensionHandlers{
		// Add the apiGroups being handled to the decoder
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			vmwarev1.GroupVersion,
			controlplanev1.GroupVersion,
			bootstrapv1.GroupVersion,
		),
	}
}

// GeneratePatches implements the HandlerFunc for the GeneratePatches hook.
// The hook adds to the response the patches we are using in Cluster API E2E tests.
// NOTE: custom RuntimeExtension must implement the body of this func according to the specific use case.
func (h *ExtensionHandlers) GeneratePatches(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("GeneratePatches is called")

	// FIXME: Everything below here will probably go away once we add the scaffolding for semantic patches.

	// By using WalkTemplates it is possible to implement patches using typed API objects, which makes code
	// easier to read and less error prone than using unstructured or working with raw json/yaml.
	// IMPORTANT: by unit testing this func/nested func properly, it is possible to prevent unexpected rollouts when patches are modified.
	topologymutation.WalkTemplates(ctx, h.decoder, req, resp, &api.Variables{}, func(ctx context.Context, obj runtime.Object, builtinVariable *patchvariables.Builtins, variables interface{}, holderRef runtimehooksv1.HolderReference) error {
		// log := ctrl.LoggerFrom(ctx) //nolint:gocritic // Not used for now, but keeping it for awareness that we can get a logger like this (with useful k/v pairs)

		vars, ok := variables.(*api.Variables)
		if !ok {
			return errors.Errorf("wrong variable type")
		}

		switch obj := obj.(type) {
		case *vmwarev1.VSphereClusterTemplate:
			if err := patchVSphereClusterTemplate(ctx, obj, builtinVariable, vars); err != nil {
				return errors.Wrapf(err, "error patching VSphereClusterTemplate")
			}
		case *controlplanev1.KubeadmControlPlaneTemplate:
			if err := patchKubeadmControlPlaneTemplate(ctx, obj, builtinVariable, vars); err != nil {
				return errors.Wrapf(err, "error patching KubeadmControlPlaneTemplate")
			}
		case *bootstrapv1.KubeadmConfigTemplate:
			// NOTE: KubeadmConfigTemplate could be linked to one or more of the existing MachineDeployment class;
			// the patchKubeadmConfigTemplate func shows how to implement patches only for KubeadmConfigTemplates
			// linked to a specific MachineDeployment class; another option is to check the holderRef value and call
			// this func or more specialized func conditionally.
			if err := patchKubeadmConfigTemplate(ctx, obj, builtinVariable, vars); err != nil {
				return errors.Wrap(err, "error patching KubeadmConfigTemplate")
			}
		case *vmwarev1.VSphereMachineTemplate:
			// NOTE: VSphereMachineTemplate could be linked to the ControlPlane or one or more of the existing MachineDeployment class;
			// the patchVSphereMachineTemplate func shows how to implement different patches for VSphereMachineTemplate
			// linked to ControlPlane or for VSphereMachineTemplate linked to MachineDeployment classes; another option
			// is to check the holderRef value and call this func or more specialized func conditionally.
			if err := patchVSphereMachineTemplate(ctx, obj, builtinVariable, vars); err != nil {
				return errors.Wrap(err, "error patching VSphereMachineTemplate")
			}
		}
		return nil
	})
}

// patchVSphereClusterTemplate patches the VSphereClusterTemplate.
func patchVSphereClusterTemplate(_ context.Context, vSphereClusterTemplate *vmwarev1.VSphereClusterTemplate, builtinVariable *patchvariables.Builtins, variables *api.Variables) error {
	_ = vSphereClusterTemplate
	_ = builtinVariable
	_ = variables
	return nil
}

// patchKubeadmControlPlaneTemplate patches the ControlPlaneTemplate.
func patchKubeadmControlPlaneTemplate(_ context.Context, kcpTemplate *controlplanev1.KubeadmControlPlaneTemplate, builtinVariable *patchvariables.Builtins, variables *api.Variables) error {
	_ = kcpTemplate
	_ = builtinVariable
	_ = variables
	return nil
}

// patchKubeadmConfigTemplate patches the KubeadmConfigTemplate.
func patchKubeadmConfigTemplate(_ context.Context, kubeadmConfigTemplate *bootstrapv1.KubeadmConfigTemplate, builtinVariable *patchvariables.Builtins, variables *api.Variables) error {
	_ = kubeadmConfigTemplate
	_ = builtinVariable
	_ = variables
	return nil
}

// patchVSphereMachineTemplate patches the VSphereMachineTemplate.
func patchVSphereMachineTemplate(_ context.Context, vSphereMachineTemplate *vmwarev1.VSphereMachineTemplate, builtinVariable *patchvariables.Builtins, variables *api.Variables) error {
	_ = vSphereMachineTemplate
	_ = builtinVariable
	_ = variables
	return nil
}

// ValidateTopology implements the HandlerFunc for the ValidateTopology hook.
func (h *ExtensionHandlers) ValidateTopology(_ context.Context, req *runtimehooksv1.ValidateTopologyRequest, resp *runtimehooksv1.ValidateTopologyResponse) {
	_ = req
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DiscoverVariables implements the HandlerFunc for the DiscoverVariables hook.
// Can be tested via Tilt:
// First terminal: kubectl proxy
// Second terminal: curl -X 'POST' 'http://127.0.0.1:8001/api/v1/namespaces/test-extension-system/services/https:test-extension-webhook-service:443/proxy/hooks.runtime.cluster.x-k8s.io/v1alpha1/discovervariables/discover-variables' -d '{"apiVersion":"hooks.runtime.cluster.x-k8s.io/v1alpha1","kind":"DiscoverVariablesRequest"}' | jq
// Should return the DiscoveryVariablesResponse.
func (h *ExtensionHandlers) DiscoverVariables(_ context.Context, _ *runtimehooksv1.DiscoverVariablesRequest, resp *runtimehooksv1.DiscoverVariablesResponse) {
	resp.Variables = api.VariableDefinitions
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}
