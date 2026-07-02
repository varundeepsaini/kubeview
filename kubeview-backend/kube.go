package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

// emptyKubePath is the sentinel for an unset kubeconfig path or home directory.
const emptyKubePath = ""

// Client wraps a Kubernetes clientset plus contextual info from the kubeconfig.
type Client struct {
	clientset kubernetes.Interface
	discovery discovery.DiscoveryInterface
	context   string
	cluster   string
}

// Node response shapes. JSON tags must match what the frontend expects in
// kubeview-frontend/src/lib/api.ts.

type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type NodeAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type NodeInfo struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	Roles            []string          `json:"roles"`
	Version          string            `json:"version"`
	OS               string            `json:"os"`
	Arch             string            `json:"arch"`
	ContainerRuntime string            `json:"containerRuntime"`
	CPU              string            `json:"cpu"`
	Memory           string            `json:"memory"`
	Pods             string            `json:"pods"`
	Labels           map[string]string `json:"labels"`
	Conditions       []NodeCondition   `json:"conditions"`
	CreatedAt        string            `json:"createdAt"`
	Age              string            `json:"age"`
	Addresses        []NodeAddress     `json:"addresses"`
}

func NewClient() (*Client, error) {
	config, kubeCtx, err := loadKubeConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Client{
		clientset: clientset,
		discovery: clientset.Discovery(),
		context:   kubeCtx.contextName,
		cluster:   kubeCtx.clusterName,
	}, nil
}

// kubeContext bundles the current context and cluster names so loadKubeConfig
// returns a single distinct type instead of two same-typed string results.
type kubeContext struct {
	contextName string
	clusterName string
}

func loadKubeConfig() (*rest.Config, kubeContext, error) {
	var emptyCtx kubeContext

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == emptyKubePath {
		if home := homedir.HomeDir(); home != emptyKubePath {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	loadingRules := new(clientcmd.ClientConfigLoadingRules)
	loadingRules.ExplicitPath = kubeconfigPath
	overrides := new(clientcmd.ConfigOverrides)
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		overrides,
	)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		err = fmt.Errorf("load kubeconfig: %w", err)

		return nil, emptyCtx, err
	}

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		err = fmt.Errorf("build rest config: %w", err)

		return nil, emptyCtx, err
	}

	ctxName := rawConfig.CurrentContext

	return restConfig, kubeContext{
		contextName: ctxName,
		clusterName: clusterNameFor(rawConfig, ctxName),
	}, nil
}

// clusterNameFor resolves the cluster bound to the current context, defaulting
// to "unknown" when it cannot be determined.
func clusterNameFor(raw clientcmdapi.Config, currentContext string) string {
	ctx, ok := raw.Contexts[currentContext]
	if ok && ctx.Cluster != emptyKubePath {
		return ctx.Cluster
	}

	return "unknown"
}

// listOptions returns a zero-value metav1.ListOptions. Built via a var
// declaration (not a composite literal) so the full option set stays at its
// documented defaults without enumerating every field.
func listOptions() metav1.ListOptions {
	var opts metav1.ListOptions

	return opts
}

func getOptions() metav1.GetOptions {
	var opts metav1.GetOptions

	return opts
}

func (c *Client) GetClusterInfo(ctx context.Context) (ClusterInfo, error) {
	var info ClusterInfo

	ver, err := c.discovery.ServerVersion()
	if err != nil {
		return info, fmt.Errorf("server version: %w", err)
	}

	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, listOptions())
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

func (c *Client) ListNamespaces(
	ctx context.Context,
) ([]corev1.Namespace, error) {
	list, err := c.clientset.CoreV1().Namespaces().List(ctx, listOptions())
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	return list.Items, nil
}

func (c *Client) ListPods(
	ctx context.Context,
	namespace string,
) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, listOptions())
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	return list.Items, nil
}

func (c *Client) GetPod(
	ctx context.Context,
	namespace, name string,
) (*corev1.Pod, error) {
	pods := c.clientset.CoreV1().Pods(namespace)

	pod, err := pods.Get(ctx, name, getOptions())
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}

	return pod, nil
}

func (c *Client) GetPodLogs(
	ctx context.Context,
	namespace, name, container string,
	tailLines int64,
) (string, error) {
	// Multi-container pods reject log requests without an explicit container
	// (the API server answers 400), so fall back to the container kubectl
	// would pick: the default-container annotation when it names a real
	// container, else the first spec container.
	if container == emptyKubePath {
		pod, err := c.GetPod(ctx, namespace, name)
		if err != nil {
			return emptyKubePath, err
		}

		container = defaultLogContainer(pod)
	}

	opts := podLogOptions(tailLines, container)
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return emptyKubePath, fmt.Errorf("open log stream: %w", err)
	}

	defer closeLogStream(stream)

	raw, err := io.ReadAll(stream)
	if err != nil {
		return emptyKubePath, fmt.Errorf("read log stream: %w", err)
	}

	return string(raw), nil
}

// defaultLogContainer picks the container a log request should target when
// the caller did not name one: the kubectl.kubernetes.io/default-container
// annotation when it names a spec container, else the first spec container.
func defaultLogContainer(pod *corev1.Pod) string {
	if annotated, ok := pod.Annotations[annotationDefaultContainer]; ok {
		for _, spec := range pod.Spec.Containers {
			if spec.Name == annotated {
				return annotated
			}
		}
	}

	if len(pod.Spec.Containers) > zeroCount {
		return pod.Spec.Containers[zeroCount].Name
	}

	return emptyKubePath
}

func closeLogStream(stream io.Closer) {
	closeErr := stream.Close()
	if closeErr != nil {
		log.Printf("close log stream: %v", closeErr)
	}
}

// podLogOptions builds the log request options, setting only the fields we
// care about and leaving the rest at their zero defaults.
func podLogOptions(tailLines int64, container string) *corev1.PodLogOptions {
	opts := new(corev1.PodLogOptions)
	opts.TailLines = &tailLines

	if container != "" {
		opts.Container = container
	}

	return opts
}

func (c *Client) ListDeployments(
	ctx context.Context,
	namespace string,
) ([]appsv1.Deployment, error) {
	deployments := c.clientset.AppsV1().Deployments(namespace)

	list, err := deployments.List(ctx, listOptions())
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	return list.Items, nil
}

func (c *Client) ListServices(
	ctx context.Context,
	namespace string,
) ([]corev1.Service, error) {
	services := c.clientset.CoreV1().Services(namespace)

	list, err := services.List(ctx, listOptions())
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	return list.Items, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	list, err := c.clientset.CoreV1().Nodes().List(ctx, listOptions())
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	return list.Items, nil
}

func (c *Client) ListEvents(
	ctx context.Context,
	namespace string,
) ([]corev1.Event, error) {
	list, err := c.clientset.CoreV1().Events(namespace).List(ctx, listOptions())
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	return list.Items, nil
}
