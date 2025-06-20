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

package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrav1 "sigs.k8s.io/cluster-api-provider-vsphere/apis/v1beta1"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/clustermodule"
	capvcontext "sigs.k8s.io/cluster-api-provider-vsphere/pkg/context"
)

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vsphereclusters,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vspheremachinetemplates,verbs=get;list;watch

// Reconciler reconciles changes for ClusterModules.
type Reconciler struct {
	Client client.Client

	ClusterModuleService clustermodule.Service
}

// NewReconciler creates a Cluster Module Reconciler with a Client and ClusterModuleService.
func NewReconciler(controllerManagerCtx *capvcontext.ControllerManagerContext) Reconciler {
	return Reconciler{
		Client:               controllerManagerCtx.Client,
		ClusterModuleService: clustermodule.NewService(controllerManagerCtx, controllerManagerCtx.Client),
	}
}

func (r Reconciler) Reconcile(ctx context.Context, clusterCtx *capvcontext.ClusterContext) (reconcile.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !clustermodule.IsClusterCompatible(clusterCtx) {
		v1beta1conditions.MarkFalse(clusterCtx.VSphereCluster, infrav1.ClusterModulesAvailableCondition, infrav1.VCenterVersionIncompatibleReason, clusterv1beta1.ConditionSeverityInfo,
			"vCenter version %s does not support cluster modules", clusterCtx.VSphereCluster.Status.VCenterVersion)
		v1beta2conditions.Set(clusterCtx.VSphereCluster, metav1.Condition{
			Type:    infrav1.VSphereClusterClusterModulesReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.VSphereClusterModulesInvalidVCenterVersionV1Beta2Reason,
			Message: fmt.Sprintf("vCenter version %s does not support cluster modules", clusterCtx.VSphereCluster.Status.VCenterVersion),
		})
		log.V(5).Info(fmt.Sprintf("vCenter version %s does not support cluster modules to implement anti affinity (vCenter >= 7 required)", clusterCtx.VSphereCluster.Status.VCenterVersion))
		return reconcile.Result{}, nil
	}

	objectMap, err := r.fetchMachineOwnerObjects(ctx, clusterCtx)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get Machine owner objects")
	}

	modErrs := []clusterModError{}

	clusterModuleSpecs := []infrav1.ClusterModule{}
	for _, mod := range clusterCtx.VSphereCluster.Spec.ClusterModules {
		// Note: We have to use := here to not overwrite log & ctx outside the for loop.
		log := log
		// It is safe to infer KubeadmControlPlane or MachineDeployment from .ControlPlane as modules
		// are only implemented for these types.
		if mod.ControlPlane {
			log = log.WithValues("KubeadmControlPlane", klog.KRef(clusterCtx.VSphereCluster.Namespace, mod.TargetObjectName), "moduleUUID", mod.ModuleUUID)
		} else {
			log = log.WithValues("MachineDeployment", klog.KRef(clusterCtx.VSphereCluster.Namespace, mod.TargetObjectName), "moduleUUID", mod.ModuleUUID)
		}
		ctx := ctrl.LoggerInto(ctx, log)

		curr := mod.TargetObjectName
		if mod.ControlPlane {
			curr = appendKCPKey(curr)
		}
		if obj, ok := objectMap[curr]; !ok {
			// Delete the cluster module as the object is marked for deletion or already deleted.
			if err := r.ClusterModuleService.Remove(ctx, clusterCtx, mod.ModuleUUID); err != nil {
				log.Error(err, "Failed to delete cluster module for object")
			}
			delete(objectMap, curr)
		} else {
			// Verify the cluster module
			exists, err := r.ClusterModuleService.DoesExist(ctx, clusterCtx, obj, mod.ModuleUUID)
			if err != nil {
				modErrs = append(modErrs, clusterModError{obj.GetName(), errors.Wrapf(err, "failed to check if cluster module %q exists", mod.ModuleUUID)})
				log.Error(err, "Failed to check if cluster module for object exists")
				// Append the module and remove it from objectMap to not create new ones instead.
				clusterModuleSpecs = append(clusterModuleSpecs, infrav1.ClusterModule{
					ControlPlane:     obj.IsControlPlane(),
					TargetObjectName: obj.GetName(),
					ModuleUUID:       mod.ModuleUUID,
				})
				delete(objectMap, curr)
				continue
			}

			// Append the module and object info to the VSphereCluster object
			// and remove it from the object map since no new cluster module
			// needs to be created.
			if exists {
				clusterModuleSpecs = append(clusterModuleSpecs, infrav1.ClusterModule{
					ControlPlane:     obj.IsControlPlane(),
					TargetObjectName: obj.GetName(),
					ModuleUUID:       mod.ModuleUUID,
				})
				delete(objectMap, curr)
			} else {
				log.V(4).Info("Module for object not found (will be created)")
			}
		}
	}

	for _, obj := range objectMap {
		// Note: We have to use := here to create a new variable and not overwrite log & ctx outside the for loop.
		log := log.WithValues(obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))
		ctx := ctrl.LoggerInto(ctx, log)

		moduleUUID, err := r.ClusterModuleService.Create(ctx, clusterCtx, obj)
		if err != nil {
			modErrs = append(modErrs, clusterModError{obj.GetName(), errors.Wrapf(err, "failed to create cluster module")})
			log.Error(err, "Failed to create cluster module for object")
			continue
		}
		// module creation was skipped
		if moduleUUID == "" {
			continue
		}
		clusterModuleSpecs = append(clusterModuleSpecs, infrav1.ClusterModule{
			ControlPlane:     obj.IsControlPlane(),
			TargetObjectName: obj.GetName(),
			ModuleUUID:       moduleUUID,
		})
	}
	clusterCtx.VSphereCluster.Spec.ClusterModules = clusterModuleSpecs

	switch {
	case len(modErrs) > 0:
		incompatibleOwnerErrs := incompatibleOwnerErrors(modErrs)
		// if cluster module creation is not possible due to incompatibility,
		// cluster creation should succeed with a warning condition
		if len(incompatibleOwnerErrs) > 0 && len(incompatibleOwnerErrs) == len(modErrs) {
			err = nil
		} else {
			err = errors.New(generateClusterModuleErrorMessage(modErrs))
		}
		v1beta1conditions.MarkFalse(clusterCtx.VSphereCluster, infrav1.ClusterModulesAvailableCondition, infrav1.ClusterModuleSetupFailedReason,
			clusterv1beta1.ConditionSeverityWarning, "%s", generateClusterModuleErrorMessage(modErrs))
		v1beta2conditions.Set(clusterCtx.VSphereCluster, metav1.Condition{
			Type:    infrav1.VSphereClusterClusterModulesReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.VSphereClusterClusterModulesNotReadyV1Beta2Reason,
			Message: generateClusterModuleErrorMessage(modErrs),
		})
		return reconcile.Result{}, err
	case len(modErrs) == 0 && len(clusterModuleSpecs) > 0:
		v1beta1conditions.MarkTrue(clusterCtx.VSphereCluster, infrav1.ClusterModulesAvailableCondition)
	default:
		v1beta1conditions.Delete(clusterCtx.VSphereCluster, infrav1.ClusterModulesAvailableCondition)
	}
	v1beta2conditions.Set(clusterCtx.VSphereCluster, metav1.Condition{
		Type:   infrav1.VSphereClusterClusterModulesReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.VSphereClusterClusterModulesReadyV1Beta2Reason,
	})
	return reconcile.Result{}, err
}

func toAffinityInput[T client.Object](c client.Client) handler.TypedMapFunc[T, ctrl.Request] {
	return func(ctx context.Context, obj T) []ctrl.Request {
		log := ctrl.LoggerFrom(ctx)

		cluster, err := util.GetClusterFromMetadata(ctx, c, metav1.ObjectMeta{
			Namespace:       obj.GetNamespace(),
			Labels:          obj.GetLabels(),
			OwnerReferences: obj.GetOwnerReferences(),
		})
		if err != nil {
			log.V(4).Error(err, "Failed to get owner Cluster")
			return nil
		}
		log = log.WithValues("Cluster", klog.KObj(cluster))
		ctx = ctrl.LoggerInto(ctx, log)

		if cluster.Spec.InfrastructureRef == nil {
			log.V(4).Error(err, "Failed to get VSphereCluster: Cluster.spec.infrastructureRef not set")
			return nil
		}

		log = log.WithValues("VSphereCluster", klog.KRef(cluster.Namespace, cluster.Spec.InfrastructureRef.Name))
		ctx = ctrl.LoggerInto(ctx, log)

		vsphereCluster := &infrav1.VSphereCluster{}
		if err := c.Get(ctx, client.ObjectKey{
			Name:      cluster.Spec.InfrastructureRef.Name,
			Namespace: cluster.Namespace,
		}, vsphereCluster); err != nil {
			log.V(4).Error(err, "Failed to get VSphereCluster")
			return nil
		}

		return []reconcile.Request{
			{NamespacedName: client.ObjectKeyFromObject(vsphereCluster)},
		}
	}
}

// PopulateWatchesOnController adds watches to the ClusterModule reconciler.
func (r Reconciler) PopulateWatchesOnController(mgr manager.Manager, controller controller.Controller) error {
	if err := controller.Watch(
		source.Kind(
			mgr.GetCache(),
			&controlplanev1.KubeadmControlPlane{},
			handler.TypedEnqueueRequestsFromMapFunc(toAffinityInput[*controlplanev1.KubeadmControlPlane](r.Client)),
			predicate.TypedFuncs[*controlplanev1.KubeadmControlPlane]{
				GenericFunc: func(event.TypedGenericEvent[*controlplanev1.KubeadmControlPlane]) bool {
					return false
				},
				UpdateFunc: func(event.TypedUpdateEvent[*controlplanev1.KubeadmControlPlane]) bool {
					return false
				},
			},
		),
	); err != nil {
		return err
	}

	return controller.Watch(
		source.Kind(
			mgr.GetCache(),
			&clusterv1.MachineDeployment{},
			handler.TypedEnqueueRequestsFromMapFunc(toAffinityInput[*clusterv1.MachineDeployment](r.Client)),
			predicate.TypedFuncs[*clusterv1.MachineDeployment]{
				GenericFunc: func(event.TypedGenericEvent[*clusterv1.MachineDeployment]) bool {
					return false
				},
				UpdateFunc: func(event.TypedUpdateEvent[*clusterv1.MachineDeployment]) bool {
					return false
				},
			},
		),
	)
}

func (r Reconciler) fetchMachineOwnerObjects(ctx context.Context, clusterCtx *capvcontext.ClusterContext) (map[string]clustermodule.Wrapper, error) {
	objects := map[string]clustermodule.Wrapper{}

	name, ok := clusterCtx.VSphereCluster.GetLabels()[clusterv1.ClusterNameLabel]
	if !ok {
		return nil, errors.Errorf("failed to get Cluster name from VSphereCluster: missing cluster name label")
	}

	labels := map[string]string{clusterv1.ClusterNameLabel: name}
	kcpList := &controlplanev1.KubeadmControlPlaneList{}
	if err := r.Client.List(
		ctx, kcpList,
		client.InNamespace(clusterCtx.VSphereCluster.GetNamespace()),
		client.MatchingLabels(labels)); err != nil {
		return nil, errors.Wrapf(err, "failed to list KubeadmControlPlane objects")
	}
	if len(kcpList.Items) > 1 {
		return nil, errors.Errorf("multiple KubeadmControlPlane objects found, expected 1, found %d", len(kcpList.Items))
	}

	if len(kcpList.Items) != 0 {
		if kcp := &kcpList.Items[0]; kcp.GetDeletionTimestamp().IsZero() {
			objects[appendKCPKey(kcp.GetName())] = clustermodule.NewWrapper(kcp)
		}
	}

	mdList := &clusterv1.MachineDeploymentList{}
	if err := r.Client.List(
		ctx, mdList,
		client.InNamespace(clusterCtx.VSphereCluster.GetNamespace()),
		client.MatchingLabels(labels)); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineDeployment objects")
	}
	for _, md := range mdList.Items {
		if md.DeletionTimestamp.IsZero() {
			objects[md.GetName()] = clustermodule.NewWrapper(md.DeepCopy())
		}
	}
	return objects, nil
}

// appendKCPKey adds the prefix "kcp" to the name of the object
// This is used to separate a single KCP object from the Machine Deployment objects
// having the same name.
func appendKCPKey(name string) string {
	return "kcp" + name
}

func incompatibleOwnerErrors(errList []clusterModError) []clusterModError {
	toReport := []clusterModError{}
	for _, e := range errList {
		if clustermodule.IsIncompatibleOwnerError(e.err) {
			toReport = append(toReport, e)
		}
	}
	return toReport
}

type clusterModError struct {
	name string
	err  error
}

func generateClusterModuleErrorMessage(errList []clusterModError) string {
	sb := strings.Builder{}
	sb.WriteString("Failed to create cluster modules for: ")

	for _, e := range errList {
		sb.WriteString(fmt.Sprintf("%s %s, ", e.name, e.err.Error()))
	}
	msg := sb.String()
	return msg[:len(msg)-2]
}
