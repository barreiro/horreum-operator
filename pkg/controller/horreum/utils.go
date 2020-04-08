package horreum

import (
	"math/rand"
	"strconv"
	"time"

	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func withDefault(custom string, def string) string {
	if custom == "" {
		return def
	}
	return custom
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
		StringData: map[string]string{
			"user":     name,
			"password": generatePassword(),
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
