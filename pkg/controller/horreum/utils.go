package horreum

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	return ifThenElse(route.Type == "http", "http://", "https://") + withDefault(route.Host, defaultHost)
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
	switch cr.Spec.Route.Type {
	case "http":
		return nil, nil
	// passthrough route must not set certs
	case "passthrough":
		return &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationPassthrough,
			InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		}, nil
	}
	tlsSecret := corev1.Secret{}
	if cr.Spec.Route.TLS != "" {
		if error := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.Route.TLS, Namespace: cr.Namespace}, &tlsSecret); error != nil {
			updateStatus(r, cr, "Error", "Cannot find secret "+route.TLS)
			return nil, error
		}
	}
	cacert := ""
	if bytes, ok := tlsSecret.Data["ca.crt"]; ok {
		cacert = string(bytes)
	}
	var termination routev1.TLSTerminationType
	switch cr.Spec.Route.Type {
	case "edge":
		termination = routev1.TLSTerminationEdge
	case "reencrypt", "":
		termination = routev1.TLSTerminationReencrypt
	default:
		log.Info("Invalid route type: " + cr.Spec.Route.Type)
		return nil, errors.New("Invalid route type: " + cr.Spec.Route.Type)
	}
	return &routev1.TLSConfig{
		Termination:                   termination,
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

func innerProtocol(route hyperfoilv1alpha1.RouteSpec) string {
	if route.Type == "http" || route.Type == "edge" {
		return "http://"
	} else {
		return "https://"
	}
}

func servicePort(route hyperfoilv1alpha1.RouteSpec, httpPort int32, httpsPort int32) corev1.ServicePort {
	if route.Type == "http" || route.Type == "edge" {
		return corev1.ServicePort{
			Name: "http",
			Port: int32(80),
			TargetPort: intstr.IntOrString{
				IntVal: httpPort,
			},
		}
	} else {
		return corev1.ServicePort{
			Name: "https",
			Port: int32(443),
			TargetPort: intstr.IntOrString{
				IntVal: httpsPort,
			},
		}
	}
}
