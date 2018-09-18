// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// The path to the pods log directory.
const podsDirectoryPath = "/var/log/pods"

// Launcher looks for new and deleted pods to create or delete one logs-source per container.
type Launcher struct {
	sources                   *config.LogSources
	sourcesByContainer        map[string]*config.LogSource
	stopped                   chan struct{}
	kubeutil                  *kubelet.KubeUtil
	dockerAddedServices       chan *service.Service
	dockerRemovedServices     chan *service.Service
	containerdAddedServices   chan *service.Service
	containerdRemovedServices chan *service.Service
}

// NewLauncher returns a new launcher.
func NewLauncher(sources *config.LogSources, services *service.Services) (*Launcher, error) {
	kubeutil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	launcher := &Launcher{
		sources:                   sources,
		sourcesByContainer:        make(map[string]*config.LogSource),
		stopped:                   make(chan struct{}),
		kubeutil:                  kubeutil,
		dockerAddedServices:       services.GetAddedServices(service.Docker),
		dockerRemovedServices:     services.GetRemovedServices(service.Docker),
		containerdAddedServices:   services.GetAddedServices(service.Containerd),
		containerdRemovedServices: services.GetRemovedServices(service.Containerd),
	}
	err = launcher.setup()
	if err != nil {
		return nil, err
	}
	return launcher, nil
}

// setup initializes the pod watcher and the tagger.
func (l *Launcher) setup() error {
	var err error
	// initialize the tagger to collect container tags
	err = tagger.Init()
	if err != nil {
		return err
	}
	return nil
}

// Start starts the launcher
func (l *Launcher) Start() {
	log.Info("Starting Kubernetes launcher")
	go l.run()
}

// Stop stops the launcher
func (l *Launcher) Stop() {
	log.Info("Stopping Kubernetes launcher")
	l.stopped <- struct{}{}
}

// run handles new and deleted pods,
// the kubernetes launcher consumes new and deleted services pushed by the autodiscovery
func (l *Launcher) run() {
	for {
		select {
		case service := <-l.dockerAddedServices:
			l.addSources(service)
		case service := <-l.dockerRemovedServices:
			l.removeSources(service)
		case service := <-l.containerdAddedServices:
			l.addSources(service)
		case service := <-l.containerdRemovedServices:
			l.removeSources(service)
		case <-l.stopped:
			return
		}
	}
}

// addSources creates a new log-source from a service by resolving the
// pod linked to the entityID of the service
func (l *Launcher) addSources(service *service.Service) {
	pod, err := l.kubeutil.GetPodForEntityID(service.GetEntityID())
	if err != nil {
		log.Warnf("Could not add source for container %v: %v", service.Identifier, err)
		return
	}
	l.addSourcesFromPod(pod)
}

// removeSources removes a new log-source from a service
func (l *Launcher) removeSources(service *service.Service) {
	containerID := service.GetEntityID()
	if source, exists := l.sourcesByContainer[containerID]; exists {
		delete(l.sourcesByContainer, containerID)
		l.sources.RemoveSource(source)
	}
}

// addSourcesFromPod creates new log-sources for each container of the pod.
// it checks if the sources already exist to avoid tailing twice the same
// container when pods have multiple containers
func (l *Launcher) addSourcesFromPod(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		containerID := container.ID
		if _, exists := l.sourcesByContainer[containerID]; exists {
			continue
		}
		source, err := l.getSource(pod, container)
		if err != nil {
			log.Warnf("Invalid configuration for pod %v, container %v: %v", pod.Metadata.Name, container.Name, err)
			continue
		}
		l.sourcesByContainer[containerID] = source
		l.sources.AddSource(source)
	}
}

// kubernetesIntegration represents the name of the integration.
const kubernetesIntegration = "kubernetes"

// getSource returns a new source for the container in pod.
func (l *Launcher) getSource(pod *kubelet.Pod, container kubelet.ContainerStatus) (*config.LogSource, error) {
	var cfg *config.LogsConfig
	if annotation := l.getAnnotation(pod, container); annotation != "" {
		configs, err := config.ParseJSON([]byte(annotation))
		if err != nil || len(configs) == 0 {
			return nil, fmt.Errorf("could not parse kubernetes annotation %v", annotation)
		}
		cfg = configs[0]
	} else {
		cfg = &config.LogsConfig{
			Source:  kubernetesIntegration,
			Service: kubernetesIntegration,
		}
	}
	cfg.Type = config.FileType
	cfg.Path = l.getPath(pod, container)
	cfg.Identifier = container.ID
	cfg.Tags = append(cfg.Tags, l.getTags(container)...)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes annotation: %v", err)
	}
	if err := cfg.Compile(); err != nil {
		return nil, fmt.Errorf("could not compile kubernetes annotation: %v", err)
	}
	return config.NewLogSource(l.getSourceName(pod, container), cfg), nil
}

// configPath refers to the configuration that can be passed over a pod annotation,
// this feature is commonly named 'ad' or 'autodicovery'.
// The pod annotation must respect the format: ad.datadoghq.com/<container_name>.logs: '[{...}]'.
const (
	configPathPrefix = "ad.datadoghq.com"
	configPathSuffix = "logs"
)

// getConfigPath returns the path of the logs-config annotation for container.
func (l *Launcher) getConfigPath(container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s.%s", configPathPrefix, container.Name, configPathSuffix)
}

// getAnnotation returns the logs-config annotation for container if present.
func (l *Launcher) getAnnotation(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	configPath := l.getConfigPath(container)
	if annotation, exists := pod.Metadata.Annotations[configPath]; exists {
		return annotation
	}
	return ""
}

// getSourceName returns the source name of the container to tail.
func (l *Launcher) getSourceName(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s", pod.Metadata.Namespace, pod.Metadata.Name, container.Name)
}

// getPath returns the path where all the logs of the container of the pod are stored.
func (l *Launcher) getPath(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s/*.log", podsDirectoryPath, pod.Metadata.UID, container.Name)
}

// getTags returns all the tags of the container
func (l *Launcher) getTags(container kubelet.ContainerStatus) []string {
	tags, _ := tagger.Tag(container.ID, true)
	return tags
}
