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

package v1

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/util/feature"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	configv1 "k8s.io/kube-scheduler/config/v1"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/utils/pointer"
)

var defaultResourceSpec = []configv1.ResourceSpec{
	{Name: string(v1.ResourceCPU), Weight: 1},
	{Name: string(v1.ResourceMemory), Weight: 1},
}

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

func pluginsNames(p *configv1.Plugins) []string {
	if p == nil {
		return nil
	}
	extensions := []configv1.PluginSet{
		p.MultiPoint,
		p.PreFilter,
		p.Filter,
		p.PostFilter,
		p.Reserve,
		p.PreScore,
		p.Score,
		p.PreBind,
		p.Bind,
		p.PostBind,
		p.Permit,
		p.QueueSort,
	}
	n := sets.NewString()
	for _, e := range extensions {
		for _, pg := range e.Enabled {
			n.Insert(pg.Name)
		}
	}
	return n.List()
}

func setDefaults_KubeSchedulerProfile(prof *configv1.KubeSchedulerProfile) {
	// Set default plugins.
	prof.Plugins = mergePlugins(getDefaultPlugins(), prof.Plugins)
	// Set default plugin configs.
	scheme := GetPluginArgConversionScheme()
	existingConfigs := sets.NewString()
	for j := range prof.PluginConfig {
		existingConfigs.Insert(prof.PluginConfig[j].Name)
		args := prof.PluginConfig[j].Args.Object
		if _, isUnknown := args.(*runtime.Unknown); isUnknown {
			continue
		}
		scheme.Default(args)
	}

	// Append default configs for plugins that didn't have one explicitly set.
	for _, name := range pluginsNames(prof.Plugins) {
		if existingConfigs.Has(name) {
			continue
		}
		gvk := configv1.SchemeGroupVersion.WithKind(name + "Args")
		args, err := scheme.New(gvk)
		if err != nil {
			// This plugin is out-of-tree or doesn't require configuration.
			continue
		}
		scheme.Default(args)
		args.GetObjectKind().SetGroupVersionKind(gvk)
		prof.PluginConfig = append(prof.PluginConfig, configv1.PluginConfig{
			Name: name,
			Args: runtime.RawExtension{Object: args},
		})
	}
}

// SetDefaults_KubeSchedulerConfiguration sets additional defaults
func SetDefaults_KubeSchedulerConfiguration(obj *configv1.KubeSchedulerConfiguration) {
	if obj.Parallelism == nil {
		obj.Parallelism = pointer.Int32Ptr(16)
	}

	if len(obj.Profiles) == 0 {
		obj.Profiles = append(obj.Profiles, configv1.KubeSchedulerProfile{})
	}
	// Only apply a default scheduler name when there is a single profile.
	// Validation will ensure that every profile has a non-empty unique name.
	if len(obj.Profiles) == 1 && obj.Profiles[0].SchedulerName == nil {
		obj.Profiles[0].SchedulerName = pointer.StringPtr(v1.DefaultSchedulerName)
	}

	// Add the default set of plugins and apply the configuration.
	for i := range obj.Profiles {
		prof := &obj.Profiles[i]
		setDefaults_KubeSchedulerProfile(prof)
	}

	if obj.PercentageOfNodesToScore == nil {
		obj.PercentageOfNodesToScore = pointer.Int32(config.DefaultPercentageOfNodesToScore)
	}

	if len(obj.LeaderElection.ResourceLock) == 0 {
		// Use lease-based leader election to reduce cost.
		// We migrated for EndpointsLease lock in 1.17 and starting in 1.20 we
		// migrated to Lease lock.
		obj.LeaderElection.ResourceLock = "leases"
	}
	if len(obj.LeaderElection.ResourceNamespace) == 0 {
		obj.LeaderElection.ResourceNamespace = configv1.SchedulerDefaultLockObjectNamespace
	}
	if len(obj.LeaderElection.ResourceName) == 0 {
		obj.LeaderElection.ResourceName = configv1.SchedulerDefaultLockObjectName
	}

	if len(obj.ClientConnection.ContentType) == 0 {
		obj.ClientConnection.ContentType = "application/vnd.kubernetes.protobuf"
	}
	// Scheduler has an opinion about QPS/Burst, setting specific defaults for itself, instead of generic settings.
	if obj.ClientConnection.QPS == 0.0 {
		obj.ClientConnection.QPS = 50.0
	}
	if obj.ClientConnection.Burst == 0 {
		obj.ClientConnection.Burst = 100
	}

	// Use the default LeaderElectionConfiguration options
	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(&obj.LeaderElection)

	if obj.PodInitialBackoffSeconds == nil {
		obj.PodInitialBackoffSeconds = pointer.Int64(1)
	}

	if obj.PodMaxBackoffSeconds == nil {
		obj.PodMaxBackoffSeconds = pointer.Int64(10)
	}

	// Enable profiling by default in the scheduler
	if obj.EnableProfiling == nil {
		obj.EnableProfiling = pointer.Bool(true)
	}

	// Enable contention profiling by default if profiling is enabled
	if *obj.EnableProfiling && obj.EnableContentionProfiling == nil {
		obj.EnableContentionProfiling = pointer.Bool(true)
	}
}

func SetDefaults_DefaultPreemptionArgs(obj *configv1.DefaultPreemptionArgs) {
	if obj.MinCandidateNodesPercentage == nil {
		obj.MinCandidateNodesPercentage = pointer.Int32Ptr(10)
	}
	if obj.MinCandidateNodesAbsolute == nil {
		obj.MinCandidateNodesAbsolute = pointer.Int32Ptr(100)
	}
}

func SetDefaults_InterPodAffinityArgs(obj *configv1.InterPodAffinityArgs) {
	if obj.HardPodAffinityWeight == nil {
		obj.HardPodAffinityWeight = pointer.Int32Ptr(1)
	}
}

func SetDefaults_VolumeBindingArgs(obj *configv1.VolumeBindingArgs) {
	if obj.BindTimeoutSeconds == nil {
		obj.BindTimeoutSeconds = pointer.Int64Ptr(600)
	}
	if len(obj.Shape) == 0 && feature.DefaultFeatureGate.Enabled(features.VolumeCapacityPriority) {
		obj.Shape = []configv1.UtilizationShapePoint{
			{
				Utilization: 0,
				Score:       0,
			},
			{
				Utilization: 100,
				Score:       int32(config.MaxCustomPriorityScore),
			},
		}
	}
}

func SetDefaults_NodeResourcesBalancedAllocationArgs(obj *configv1.NodeResourcesBalancedAllocationArgs) {
	if len(obj.Resources) == 0 {
		obj.Resources = defaultResourceSpec
		return
	}
	// If the weight is not set or it is explicitly set to 0, then apply the default weight(1) instead.
	for i := range obj.Resources {
		if obj.Resources[i].Weight == 0 {
			obj.Resources[i].Weight = 1
		}
	}
}

func SetDefaults_PodTopologySpreadArgs(obj *configv1.PodTopologySpreadArgs) {
	if obj.DefaultingType == "" {
		obj.DefaultingType = configv1.SystemDefaulting
	}
}

func SetDefaults_NodeResourcesFitArgs(obj *configv1.NodeResourcesFitArgs) {
	if obj.ScoringStrategy == nil {
		obj.ScoringStrategy = &configv1.ScoringStrategy{
			Type:      configv1.ScoringStrategyType(config.LeastAllocated),
			Resources: defaultResourceSpec,
		}
	}
	if len(obj.ScoringStrategy.Resources) == 0 {
		// If no resources specified, use the default set.
		obj.ScoringStrategy.Resources = append(obj.ScoringStrategy.Resources, defaultResourceSpec...)
	}
	for i := range obj.ScoringStrategy.Resources {
		if obj.ScoringStrategy.Resources[i].Weight == 0 {
			obj.ScoringStrategy.Resources[i].Weight = 1
		}
	}
}
