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

// Package lifecycle contains the handlers for the lifecycle hooks.
//
// When implementing custom RuntimeExtension, it is only required to expose the required HandlerFunc with the
// signature defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
package lifecycle

import (
	"context"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ExtensionHandlers provides a common struct shared across the lifecycle hook handlers; this is convenient
// because it allows to easily provide a controller runtime client to the handler functions if necessary.
// NOTE: it is not mandatory to use a ExtensionHandlers struct in custom RuntimeExtension, what is important
// is to expose the required HandlerFunc with the signatures defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
type ExtensionHandlers struct {
}

// NewExtensionHandlers returns a ExtensionHandlers for the lifecycle hooks handlers.
func NewExtensionHandlers() *ExtensionHandlers {
	return &ExtensionHandlers{}
}

// DoBeforeClusterCreate implements the HandlerFunc for the BeforeClusterCreate hook.
func (m *ExtensionHandlers) DoBeforeClusterCreate(ctx context.Context, req *runtimehooksv1.BeforeClusterCreateRequest, resp *runtimehooksv1.BeforeClusterCreateResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BeforeClusterCreate is called")
	_ = req
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoBeforeClusterUpgrade implements the HandlerFunc for the BeforeClusterUpgrade hook.
func (m *ExtensionHandlers) DoBeforeClusterUpgrade(ctx context.Context, req *runtimehooksv1.BeforeClusterUpgradeRequest, resp *runtimehooksv1.BeforeClusterUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BeforeClusterUpgrade is called")
	_ = req
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoAfterControlPlaneInitialized implements the HandlerFunc for the AfterControlPlaneInitialized hook.
func (m *ExtensionHandlers) DoAfterControlPlaneInitialized(ctx context.Context, req *runtimehooksv1.AfterControlPlaneInitializedRequest, resp *runtimehooksv1.AfterControlPlaneInitializedResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("AfterControlPlaneInitialized is called")
	_ = req
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoAfterControlPlaneUpgrade implements the HandlerFunc for the AfterControlPlaneUpgrade hook.
func (m *ExtensionHandlers) DoAfterControlPlaneUpgrade(ctx context.Context, req *runtimehooksv1.AfterControlPlaneUpgradeRequest, resp *runtimehooksv1.AfterControlPlaneUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("AfterControlPlaneUpgrade is called")
	_ = req
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoAfterClusterUpgrade implements the HandlerFunc for the AfterClusterUpgrade hook.
func (m *ExtensionHandlers) DoAfterClusterUpgrade(ctx context.Context, request *runtimehooksv1.AfterClusterUpgradeRequest, resp *runtimehooksv1.AfterClusterUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("AfterClusterUpgrade is called")
	_ = request
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoBeforeClusterDelete implements the HandlerFunc for the BeforeClusterDelete hook.
func (m *ExtensionHandlers) DoBeforeClusterDelete(ctx context.Context, request *runtimehooksv1.BeforeClusterDeleteRequest, resp *runtimehooksv1.BeforeClusterDeleteResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BeforeClusterDelete is called")
	_ = request
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}
