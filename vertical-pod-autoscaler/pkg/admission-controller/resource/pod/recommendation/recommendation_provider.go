/*
Copyright 2018 The Kubernetes Authors.

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

package recommendation

import (
	core "k8s.io/api/core/v1"
	vpa_types "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/limitrange"
	vpa_api_util "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/vpa"
	"k8s.io/klog"
)

// Provider gets current recommendation, annotations and vpaName for the given pod.
type Provider interface {
	GetContainersResourcesForPod(pod *core.Pod, vpa *vpa_types.VerticalPodAutoscaler) ([]vpa_api_util.ContainerResources, vpa_api_util.ContainerToAnnotationsMap, error)
}

type recommendationProvider struct {
	limitsRangeCalculator   limitrange.LimitRangeCalculator
	recommendationProcessor vpa_api_util.RecommendationProcessor
}

// NewProvider constructs the recommendation provider that can be used to determine recommendations for pods.
func NewProvider(calculator limitrange.LimitRangeCalculator,
	recommendationProcessor vpa_api_util.RecommendationProcessor) Provider {
	return &recommendationProvider{
		limitsRangeCalculator:   calculator,
		recommendationProcessor: recommendationProcessor,
	}
}

// GetContainersResources returns the recommended resources for each container in the given pod in the same order they are specified in the pod.Spec.
func GetContainersResources(pod *core.Pod, podRecommendation vpa_types.RecommendedPodResources) []vpa_api_util.ContainerResources {
	resources := make([]vpa_api_util.ContainerResources, len(pod.Spec.Containers))
	for i, container := range pod.Spec.Containers {
		recommendation := vpa_api_util.GetRecommendationForContainer(container.Name, &podRecommendation)
		if recommendation == nil {
			klog.V(2).Infof("no matching recommendation found for container %s", container.Name)
			continue
		}
		resources[i].Requests = recommendation.Target
		if resources[i].Requests.Cpu().Cmp(*container.Resources.Limits.Cpu()) > 0 {
			resources[i].Requests[core.ResourceCPU] = *container.Resources.Limits.Cpu()
		}
		if resources[i].Requests.Memory().Cmp(*container.Resources.Limits.Memory()) > 0 {
			resources[i].Requests[core.ResourceMemory] = *container.Resources.Limits.Memory()
		}
	}
	return resources
}

// GetContainersResourcesForPod returns recommended request for a given pod and associated annotations.
// The returned slice corresponds 1-1 to containers in the Pod.
func (p *recommendationProvider) GetContainersResourcesForPod(pod *core.Pod, vpa *vpa_types.VerticalPodAutoscaler) ([]vpa_api_util.ContainerResources, vpa_api_util.ContainerToAnnotationsMap, error) {
	if vpa == nil || pod == nil {
		klog.V(2).Infof("can't calculate recommendations, one of vpa(%+v), pod(%+v) is nil", vpa, pod)
		return nil, nil, nil
	}
	klog.V(2).Infof("updating requirements for pod %s.", pod.Name)

	var annotations vpa_api_util.ContainerToAnnotationsMap
	recommendedPodResources := &vpa_types.RecommendedPodResources{}

	if vpa.Status.Recommendation != nil {
		var err error
		recommendedPodResources, annotations, err = p.recommendationProcessor.Apply(vpa.Status.Recommendation, vpa.Spec.ResourcePolicy, vpa.Status.Conditions, pod)
		if err != nil {
			klog.V(2).Infof("cannot process recommendation for pod %s", pod.Name)
			return nil, annotations, err
		}
	}
	containerResources := GetContainersResources(pod, *recommendedPodResources)
	return containerResources, annotations, nil
}
