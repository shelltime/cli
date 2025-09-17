package model

import "context"

// KubernetesApp handles Kubernetes configuration files
type KubernetesApp struct {
	BaseApp
}

func NewKubernetesApp() DotfileApp {
	return &KubernetesApp{}
}

func (k *KubernetesApp) Name() string {
	return "kubernetes"
}

func (k *KubernetesApp) GetConfigPaths() []string {
	return []string{
		"~/.kube/config",
	}
}

func (k *KubernetesApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return k.CollectFromPaths(ctx, k.Name(), k.GetConfigPaths(), &skipIgnored)
}
