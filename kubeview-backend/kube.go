package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Client wraps a Kubernetes clientset plus contextual info from the kubeconfig.
type Client struct {
	clientset kubernetes.Interface
	discovery discovery.DiscoveryInterface
	context   string
	cluster   string
}

func NewClient() (*Client, error) {
	config, contextName, clusterName, err := loadKubeConfig()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}
	return &Client{
		clientset: cs,
		discovery: cs.Discovery(),
		context:   contextName,
		cluster:   clusterName,
	}, nil
}

func loadKubeConfig() (*rest.Config, string, string, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	overrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return nil, "", "", fmt.Errorf("load kubeconfig: %w", err)
	}

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, "", "", fmt.Errorf("build rest config: %w", err)
	}

	currentContext := rawConfig.CurrentContext
	clusterName := "unknown"
	if ctx, ok := rawConfig.Contexts[currentContext]; ok && ctx.Cluster != "" {
		clusterName = ctx.Cluster
	}

	return restConfig, currentContext, clusterName, nil
}

func (c *Client) GetClusterInfo(ctx context.Context) (ClusterInfo, error) {
	var info ClusterInfo
	ver, err := c.discovery.ServerVersion()
	if err != nil {
		return info, fmt.Errorf("server version: %w", err)
	}
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return info, fmt.Errorf("list nodes: %w", err)
	}
	return ClusterInfo{
		Version:     ver.GitVersion,
		Platform:    ver.Platform,
		NodeCount:   len(nodes.Items),
		Context:     c.context,
		ClusterName: c.cluster,
	}, nil
}

func (c *Client) ListNamespaces(ctx context.Context) ([]corev1.Namespace, error) {
	list, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	return c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetPodLogs(ctx context.Context, namespace, name, container string, tailLines int64) (string, error) {
	opts := &corev1.PodLogOptions{TailLines: &tailLines}
	if container != "" {
		opts.Container = container
	}
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	b, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	list, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	list, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	list, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	list, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
