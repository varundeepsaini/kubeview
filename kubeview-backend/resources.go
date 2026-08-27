package main

import (
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// Response shapes — JSON tags must match what the frontend expects in
// kubeview-frontend/src/lib/api.ts. Any drift here breaks the dashboard.

type ConfigMap struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Age       string            `json:"age"`
	Labels    map[string]string `json:"labels"`
	Keys      []string          `json:"keys"`
}

type Secret struct {
	Name        string         `json:"name"`
	Namespace   string         `json:"namespace"`
	Type        string         `json:"type"`
	DataLengths map[string]int `json:"dataLengths"`
	Age         string         `json:"age"`
}

type IngressRule struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Service string `json:"service"`
	Port    string `json:"port"`
}

type Ingress struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Class     string        `json:"class"`
	Age       string        `json:"age"`
	Rules     []IngressRule `json:"rules"`
	Addresses []string      `json:"addresses"`
}

func transformConfigMap(item corev1.ConfigMap) ConfigMap {
	keys := make([]string, zeroCount, len(item.Data)+len(item.BinaryData))
	for key := range item.Data {
		keys = append(keys, key)
	}

	for key := range item.BinaryData {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return ConfigMap{
		Name:      item.Name,
		Namespace: item.Namespace,
		Age:       getAge(item.CreationTimestamp),
		Labels:    emptyIfNil(item.Labels),
		Keys:      keys,
	}
}

func transformSecret(item corev1.Secret) Secret {
	lengths := make(map[string]int, len(item.Data))
	for key, value := range item.Data {
		lengths[key] = len(value)
	}

	return Secret{
		Name:        item.Name,
		Namespace:   item.Namespace,
		Type:        string(item.Type),
		DataLengths: lengths,
		Age:         getAge(item.CreationTimestamp),
	}
}

func transformIngress(item networkingv1.Ingress) Ingress {
	class := valueNoneBrackets
	if item.Spec.IngressClassName != nil {
		class = *item.Spec.IngressClassName
	}

	return Ingress{
		Name:      item.Name,
		Namespace: item.Namespace,
		Class:     class,
		Age:       getAge(item.CreationTimestamp),
		Rules:     ingressRules(item.Spec.Rules),
		Addresses: ingressAddresses(item.Status.LoadBalancer.Ingress),
	}
}

// ingressRules flattens the per-host HTTP paths into one rule entry per path,
// skipping host-only rules that carry no HTTP block.
func ingressRules(specRules []networkingv1.IngressRule) []IngressRule {
	rules := []IngressRule{}

	for _, rule := range specRules {
		if rule.HTTP == nil {
			continue
		}

		for _, path := range rule.HTTP.Paths {
			rules = append(rules, ingressPathRule(rule.Host, path))
		}
	}

	return rules
}

// ingressPathRule renders one HTTP path: service backends yield the service
// name and its port (by name when set, otherwise by number), resource
// backends yield Kind/Name with no port.
func ingressPathRule(
	host string,
	path networkingv1.HTTPIngressPath,
) IngressRule {
	service := "<resource>"
	port := emptyString

	if path.Backend.Service != nil {
		service = path.Backend.Service.Name
		port = path.Backend.Service.Port.Name

		if port == emptyString {
			port = strconv.Itoa(int(path.Backend.Service.Port.Number))
		}
	} else if path.Backend.Resource != nil {
		resource := path.Backend.Resource
		service = resource.Kind + "/" + resource.Name
	}

	return IngressRule{
		Host:    host,
		Path:    path.Path,
		Service: service,
		Port:    port,
	}
}

func ingressAddresses(
	ingress []networkingv1.IngressLoadBalancerIngress,
) []string {
	addresses := []string{}

	for _, entry := range ingress {
		if entry.IP != emptyString {
			addresses = append(addresses, entry.IP)
		} else if entry.Hostname != emptyString {
			addresses = append(addresses, entry.Hostname)
		}
	}

	return addresses
}
