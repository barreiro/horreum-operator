package horreum

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func withDefault(custom string, def string) string {
	if custom == "" {
		return def
	}
	return custom
}

func ifThenElse(condition bool, then string, els string) string {
	if condition {
		return then
	}
	return els
}

func url(route hyperfoilv1alpha1.RouteSpec, defaultHost string) string {
	return ifThenElse(route.TLS == "", "http://", "https://") + withDefault(route.Host, defaultHost)
}

func withDefaultInt(custom int32, def int32) string {
	if custom == 0 {
		return strconv.Itoa(int(def))
	}
	return strconv.Itoa(int(custom))
}

func newSecret(cr *hyperfoilv1alpha1.Horreum, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
		},
		Type: corev1.SecretTypeBasicAuth,
		StringData: map[string]string{
			corev1.BasicAuthUsernameKey: name,
			corev1.BasicAuthPasswordKey: generatePassword(),
		},
	}
}

func generatePassword() string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	length := 16
	buf := make([]rune, length)
	for i := range buf {
		buf[i] = chars[rand.Intn(len(chars))]
	}
	return string(buf)
}

func secretEnv(name string, secret string, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				Key:      key,
				Optional: &[]bool{false}[0],
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret,
				},
			},
		},
	}
}

func tls(r *ReconcileHorreum, cr *hyperfoilv1alpha1.Horreum, route hyperfoilv1alpha1.RouteSpec) (*routev1.TLSConfig, error) {
	if route.TLS == "" {
		return nil, nil
	}
	tlsSecret := corev1.Secret{}
	if error := r.client.Get(context.TODO(), types.NamespacedName{Name: route.TLS, Namespace: cr.Namespace}, &tlsSecret); error != nil {
		updateStatus(r, cr, "Error", "Cannot find secret "+route.TLS)
		return nil, error
	}
	cacert := ""
	if bytes, ok := tlsSecret.Data["ca.crt"]; ok {
		cacert = string(bytes)
	}
	return &routev1.TLSConfig{
		Termination:                   routev1.TLSTerminationEdge,
		InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		Certificate:                   string(tlsSecret.Data[corev1.TLSCertKey]),
		Key:                           string(tlsSecret.Data[corev1.TLSPrivateKeyKey]),
		CACertificate:                 cacert,
	}, nil
}

func route(route hyperfoilv1alpha1.RouteSpec, suffix string, cr *hyperfoilv1alpha1.Horreum, r *ReconcileHorreum) (*routev1.Route, error) {
	subdomain := ""
	if route.Host == "" {
		subdomain = cr.Name + suffix
	}
	tls, err := tls(r, cr, route)
	if err != nil {
		return nil, err
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + suffix,
			Namespace: cr.Namespace,
		},
		Spec: routev1.RouteSpec{
			Host:      route.Host,
			Subdomain: subdomain,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: cr.Name + suffix,
			},
			TLS: tls,
		},
	}, nil
}
