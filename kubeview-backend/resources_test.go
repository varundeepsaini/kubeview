package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// The "rt" prefix avoids collisions with constants in sibling test files.
const (
	rtPathConfigMaps   = "/api/configmaps"
	rtPathSecrets      = "/api/secrets"
	rtPathIngresses    = "/api/ingresses"
	rtPathStatefulSets = "/api/statefulsets"
	rtPathDaemonSets   = "/api/daemonsets"

	rtHost        = "example.com"
	rtSvcWeb      = "web"
	rtPortHTTP    = "http"
	rtPathRoot    = "/"
	rtClassNone   = "<none>"
	rtSecretValue = "s3cr3t-value"

	rtMsgInternal = "Internal server error"
	rtLblRulesLen = "rules len"

	rtN0 int32 = 0
	rtN1 int32 = 1
	rtN2 int32 = 2
	rtN3 int32 = 3

	rtSeedPerNamespace = 5
)

// --- fixture builders ---

func newServiceBackendPath(
	path, svc string,
	port networkingv1.ServiceBackendPort,
) networkingv1.HTTPIngressPath {
	p := new(networkingv1.HTTPIngressPath)
	p.Path = path
	p.Backend.Service = new(networkingv1.IngressServiceBackend)
	p.Backend.Service.Name = svc
	p.Backend.Service.Port = port

	return *p
}

func newIngress(
	ns string,
	rules ...networkingv1.IngressRule,
) *networkingv1.Ingress {
	ing := new(networkingv1.Ingress)
	ing.Name = ttSA
	ing.Namespace = ns
	ing.Spec.Rules = rules

	return ing
}

func newHTTPRule(
	host string,
	paths ...networkingv1.HTTPIngressPath,
) networkingv1.IngressRule {
	rule := new(networkingv1.IngressRule)
	rule.Host = host
	rule.HTTP = new(networkingv1.HTTPIngressRuleValue)
	rule.HTTP.Paths = paths

	return *rule
}

// --- transformConfigMap ---

func TestTransformConfigMap_KeysSortedAcrossDataAndBinaryData(t *testing.T) {
	t.Parallel()

	cm := new(corev1.ConfigMap)
	cm.Name = ttConfig
	cm.Namespace = ttDefault
	cm.Data = map[string]string{ttSC: ttSOk, ttSA: ttSOk}
	cm.BinaryData = map[string][]byte{ttSB: {ttZeroNum}}

	got := transformConfigMap(*cm)
	wantEq(t, ttLblName, got.Name, ttConfig)
	wantEq(t, "keys len", len(got.Keys), ttN3)
	wantEq(t, "keys[0]", got.Keys[ttZeroNum], ttSA)
	wantEq(t, "keys[1]", got.Keys[ttN1], ttSB)
	wantEq(t, "keys[2]", got.Keys[ttN2], ttSC)
}

func TestTransformConfigMap_NilLabelsSerializeAsEmptyObject(t *testing.T) {
	t.Parallel()

	cm := new(corev1.ConfigMap)
	cm.Name = ttConfig

	b := mustMarshal(t, transformConfigMap(*cm))
	if !strings.Contains(string(b), ttSLabels) {
		t.Fatalf("expected labels:{} got: %s", b)
	}
}

// --- transformSecret ---

func TestTransformSecret_ExposesLengthsNotValues(t *testing.T) {
	t.Parallel()

	sec := new(corev1.Secret)
	sec.Name = ttSSecret
	sec.Namespace = ttDefault
	sec.Type = corev1.SecretTypeOpaque
	sec.Data = map[string][]byte{ttSA: []byte(rtSecretValue)}

	got := transformSecret(*sec)
	wantEq(t, ttLblName, got.Name, ttSSecret)
	wantEq(t, "type", got.Type, string(corev1.SecretTypeOpaque))
	wantEq(t, "dataLengths[a]", got.DataLengths[ttSA], len(rtSecretValue))

	b := mustMarshal(t, got)
	if strings.Contains(string(b), rtSecretValue) {
		t.Fatalf("secret value leaked into JSON: %s", b)
	}
}

func TestTransformSecret_EmptyData(t *testing.T) {
	t.Parallel()

	sec := new(corev1.Secret)
	sec.Name = ttSSecret

	got := transformSecret(*sec)
	wantEq(t, "dataLengths len", len(got.DataLengths), ttZeroNum)
}

// --- transformIngress ---

func TestTransformIngress_PortByNumberAndByName(t *testing.T) {
	t.Parallel()

	byNumber := networkingv1.ServiceBackendPort{Name: ttEmptyStr, Number: ttN80}
	byName := networkingv1.ServiceBackendPort{
		Name:   rtPortHTTP,
		Number: ttZeroNum,
	}
	ing := newIngress(ttDefault, newHTTPRule(rtHost,
		newServiceBackendPath(rtPathRoot, rtSvcWeb, byNumber),
		newServiceBackendPath(rtPathRoot, rtSvcWeb, byName),
	))
	class := ttSNginx
	ing.Spec.IngressClassName = &class

	got := transformIngress(*ing)
	wantEq(t, "class", got.Class, ttSNginx)
	wantEq(t, rtLblRulesLen, len(got.Rules), ttN2)
	wantEq(t, "rules[0].Host", got.Rules[ttZeroNum].Host, rtHost)
	wantEq(t, "rules[0].Service", got.Rules[ttZeroNum].Service, rtSvcWeb)
	wantEq(t, "rules[0].Port", got.Rules[ttZeroNum].Port, "80")
	wantEq(t, "rules[1].Port", got.Rules[ttN1].Port, rtPortHTTP)
}

func TestTransformIngress_HostOnlyRuleWithNilHTTPDoesNotPanic(t *testing.T) {
	t.Parallel()

	rule := new(networkingv1.IngressRule)
	rule.Host = rtHost
	ing := newIngress(ttDefault, *rule)

	got := transformIngress(*ing)
	wantEq(t, "class", got.Class, rtClassNone)
	wantEq(t, rtLblRulesLen, len(got.Rules), ttZeroNum)
}

func TestTransformIngress_ResourceBackendDoesNotPanic(t *testing.T) {
	t.Parallel()

	path := new(networkingv1.HTTPIngressPath)
	path.Path = rtPathRoot
	path.Backend.Resource = new(corev1.TypedLocalObjectReference)
	path.Backend.Resource.Kind = "StorageBucket"
	path.Backend.Resource.Name = ttSB
	ing := newIngress(ttDefault, newHTTPRule(rtHost, *path))

	got := transformIngress(*ing)
	wantEq(t, rtLblRulesLen, len(got.Rules), ttN1)

	rule := got.Rules[ttZeroNum]
	wantEq(t, "rules[0].Service", rule.Service, "StorageBucket/b")
	wantEq(t, "rules[0].Port", rule.Port, ttEmptyStr)
}

func TestTransformIngress_LoadBalancerAddresses(t *testing.T) {
	t.Parallel()

	ing := newIngress(ttDefault)
	byIP := new(networkingv1.IngressLoadBalancerIngress)
	byIP.IP = ttClusterIPAddr
	byHost := new(networkingv1.IngressLoadBalancerIngress)
	byHost.Hostname = rtHost
	lbIngress := []networkingv1.IngressLoadBalancerIngress{*byIP, *byHost}
	ing.Status.LoadBalancer.Ingress = lbIngress

	got := transformIngress(*ing)
	wantEq(t, "addresses len", len(got.Addresses), ttN2)
	wantEq(t, "addresses[0]", got.Addresses[ttZeroNum], ttClusterIPAddr)
	wantEq(t, "addresses[1]", got.Addresses[ttN1], rtHost)
}

// --- transformStatefulSet ---

func TestTransformStatefulSet_DesiredComesFromSpecReplicas(t *testing.T) {
	t.Parallel()

	desired := rtN3
	sts := new(appsv1.StatefulSet)
	sts.Name = ttSA
	sts.Namespace = ttDefault
	sts.Spec.ServiceName = ttSvc
	sts.Spec.Replicas = &desired
	sts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	sts.Status.Replicas = rtN0
	sts.Status.ReadyReplicas = rtN0

	got := transformStatefulSet(*sts)
	wantEq(t, "desiredReplicas", got.DesiredReplicas, rtN3)
	wantEq(t, "replicas", got.Replicas, rtN0)
	wantEq(t, "serviceName", got.ServiceName, ttSvc)
	wantEq(t, "strategy", got.Strategy, ttRollingUpdate)
}

func TestTransformStatefulSet_NilSpecReplicasDefaultsDesiredTo0(t *testing.T) {
	t.Parallel()

	sts := new(appsv1.StatefulSet)
	sts.Name = ttSA

	got := transformStatefulSet(*sts)
	wantEq(t, "desiredReplicas", got.DesiredReplicas, rtN0)
}

// --- transformDaemonSet ---

func TestTransformDaemonSet_StatusCounters(t *testing.T) {
	t.Parallel()

	ds := new(appsv1.DaemonSet)
	ds.Name = ttSA
	ds.Namespace = ttDefault
	ds.Status.DesiredNumberScheduled = rtN3
	ds.Status.CurrentNumberScheduled = rtN2
	ds.Status.NumberReady = rtN1
	ds.Status.UpdatedNumberScheduled = rtN2
	ds.Status.NumberAvailable = rtN1

	got := transformDaemonSet(*ds)
	wantEq(t, "desired", got.Desired, rtN3)
	wantEq(t, "current", got.Current, rtN2)
	wantEq(t, "ready", got.Ready, rtN1)
	wantEq(t, "updated", got.Updated, rtN2)
	wantEq(t, "available", got.Available, rtN1)
}

// --- handlers ---

func TestHandle_ConfigMaps(t *testing.T) {
	t.Parallel()

	cm := new(corev1.ConfigMap)
	cm.Name = ttConfig
	cm.Namespace = htNSDefault
	cm.Data = map[string]string{ttSA: ttSOk}

	srv, _ := newTestServer(t, nil, cm)

	var out []ConfigMap

	res := getJSON(t, srv, rtPathConfigMaps+"?namespace="+htNSDefault, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne || out[htFirst].Name != ttConfig {
		t.Fatalf("configmaps = %+v", out)
	}
}

func TestHandle_Secrets_DoNotExposeValues(t *testing.T) {
	t.Parallel()

	sec := new(corev1.Secret)
	sec.Name = ttSSecret
	sec.Namespace = htNSDefault
	sec.Type = corev1.SecretTypeOpaque
	sec.Data = map[string][]byte{ttSA: []byte(rtSecretValue)}

	srv, _ := newTestServer(t, nil, sec)

	var out []Secret

	res := getJSON(t, srv, rtPathSecrets, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne ||
		out[htFirst].DataLengths[ttSA] != len(rtSecretValue) {
		t.Fatalf("secrets = %+v", out)
	}

	if strings.Contains(string(res.body), rtSecretValue) {
		t.Fatalf("secret value leaked in response: %s", res.body)
	}
}

func TestRouter_SecretRevealRouteAbsent(t *testing.T) {
	t.Parallel()

	sec := new(corev1.Secret)
	sec.Name = ttSSecret
	sec.Namespace = htNSDefault
	sec.Data = map[string][]byte{ttSA: []byte(rtSecretValue)}

	srv, _ := newTestServer(t, nil, sec)

	res := httpGet(t, srv.URL+rtPathSecrets+"/"+htNSDefault+"/"+ttSSecret)
	if res.statusCode != htStatusNotF {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

func TestHandle_Ingresses(t *testing.T) {
	t.Parallel()

	hostOnly := new(networkingv1.IngressRule)
	hostOnly.Host = rtHost
	byNumber := networkingv1.ServiceBackendPort{Name: ttEmptyStr, Number: ttN80}
	webPath := newServiceBackendPath(rtPathRoot, rtSvcWeb, byNumber)
	ing := newIngress(htNSDefault,
		*hostOnly,
		newHTTPRule(rtHost, webPath),
	)

	srv, _ := newTestServer(t, nil, ing)

	var out []Ingress

	res := getJSON(t, srv, rtPathIngresses, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne ||
		len(out[htFirst].Rules) != htOne ||
		out[htFirst].Rules[htFirst].Service != rtSvcWeb {
		t.Fatalf("ingresses = %+v", out)
	}
}

func TestHandle_StatefulSets(t *testing.T) {
	t.Parallel()

	desired := rtN3
	sts := new(appsv1.StatefulSet)
	sts.Name = ttSA
	sts.Namespace = htNSDefault
	sts.Spec.Replicas = &desired

	srv, _ := newTestServer(t, nil, sts)

	var out []StatefulSet

	res := getJSON(t, srv, rtPathStatefulSets, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne || out[htFirst].DesiredReplicas != rtN3 {
		t.Fatalf("statefulsets = %+v", out)
	}
}

func TestHandle_DaemonSets(t *testing.T) {
	t.Parallel()

	ds := new(appsv1.DaemonSet)
	ds.Name = ttSA
	ds.Namespace = htNSDefault
	ds.Status.DesiredNumberScheduled = rtN2

	srv, _ := newTestServer(t, nil, ds)

	var out []DaemonSet

	res := getJSON(t, srv, rtPathDaemonSets, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne || out[htFirst].Desired != rtN2 {
		t.Fatalf("daemonsets = %+v", out)
	}
}

func TestHandle_NewResources_ListErrorReturns500(t *testing.T) {
	t.Parallel()

	cases := []struct {
		resource string
		path     string
	}{
		{resource: "configmaps", path: rtPathConfigMaps},
		{resource: "secrets", path: rtPathSecrets},
		{resource: "ingresses", path: rtPathIngresses},
		{resource: "statefulsets", path: rtPathStatefulSets},
		{resource: "daemonsets", path: rtPathDaemonSets},
	}

	for _, testCase := range cases {
		t.Run(testCase.resource, func(t *testing.T) {
			t.Parallel()

			assertListError500(t, testCase.resource, testCase.path)
		})
	}
}

// assertListError500 asserts a failing list yields the generic 500 body
// without leaking the underlying error detail.
func assertListError500(t *testing.T, resource, path string) {
	t.Helper()

	srv, c := newTestServer(t, nil)
	injectResourceError(t, c.clientset, htVerbList, resource, errBoom)

	res := httpGet(t, srv.URL+path)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}

	var e errResp

	uerr := json.Unmarshal(res.body, &e)
	if uerr != nil {
		t.Fatalf(htMsgDecode, uerr)
	}

	if e.Status != htStatusErr || e.Error != rtMsgInternal {
		t.Fatalf("error body = %+v", e)
	}

	if strings.Contains(string(res.body), "boom") {
		t.Fatalf("5xx body leaked internal detail: %s", res.body)
	}
}

// namedItem decodes the identity fields common to every response shape.
type namedItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// rtSeedObjects builds one object of each new resource type per namespace.
func rtSeedObjects(namespaces ...string) []runtime.Object {
	total := rtSeedPerNamespace * len(namespaces)
	objs := make([]runtime.Object, ttZeroNum, total)

	for _, ns := range namespaces {
		cm := new(corev1.ConfigMap)
		cm.Name = ttSA
		cm.Namespace = ns

		sec := new(corev1.Secret)
		sec.Name = ttSA
		sec.Namespace = ns

		sts := new(appsv1.StatefulSet)
		sts.Name = ttSA
		sts.Namespace = ns

		ds := new(appsv1.DaemonSet)
		ds.Name = ttSA
		ds.Namespace = ns

		objs = append(objs, cm, sec, newIngress(ns), sts, ds)
	}

	return objs
}

func TestHandle_NewResources_NamespaceFilter(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(
		t, nil,
		rtSeedObjects(htNSDefault, htNSKubeSystem)...,
	)

	paths := []string{
		rtPathConfigMaps,
		rtPathSecrets,
		rtPathIngresses,
		rtPathStatefulSets,
		rtPathDaemonSets,
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			assertNamespaceFilter(t, srv, path)
		})
	}
}

// assertNamespaceFilter asserts an endpoint lists both seeded namespaces
// unfiltered and narrows with the namespace query param.
func assertNamespaceFilter(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()

	var all []namedItem

	res := getJSON(t, srv, path, &all)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(all) != htTwo {
		t.Fatalf("unfiltered = %+v", all)
	}

	var filtered []namedItem

	res = getJSON(t, srv, path+"?namespace="+htNSKubeSystem, &filtered)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(filtered) != htOne ||
		filtered[htFirst].Namespace != htNSKubeSystem {
		t.Fatalf("filtered = %+v", filtered)
	}
}
