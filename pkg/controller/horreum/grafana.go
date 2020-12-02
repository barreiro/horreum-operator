package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func grafanaPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	keycloakURL := keycloakURL(cr)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":     cr.Name,
				"service": "grafana",
			},
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &[]int64{0}[0],
			InitContainers: []corev1.Container{
				{
					Name:            "set-secret",
					Image:           appImage(cr),
					ImagePullPolicy: corev1.PullAlways,
					Command: []string{
						"sh", "-x", "-c", `
							export KC_URL='` + keycloakURL + `'
							export TOKEN=$$(curl -s $KC_URL/auth/realms/master/protocol/openid-connect/token -X POST -H 'content-type: application/x-www-form-urlencoded' -d 'username=$(KEYCLOAK_USER)&password=$(KEYCLOAK_PASSWORD)&grant_type=password&client_id=admin-cli' | jq -r .access_token)
							export CLIENTID=$$(curl -s $KC_URL/auth/admin/realms/horreum/clients -H 'Authorization: Bearer '$TOKEN | jq -r '.[] | select(.clientId=="grafana") | .id')
							export CLIENTSECRET=$$(curl -s $KC_URL/auth/admin/realms/horreum/clients/$CLIENTID/client-secret -X POST -H 'Authorization: Bearer '$TOKEN | jq -r '.value')
							[ -n "$CLIENTSECRET" ] || exit 1;
							echo $CLIENTSECRET > /etc/grafana/imports/clientsecret
						`,
					},
					Env: []corev1.EnvVar{
						secretEnv("KEYCLOAK_USER", keycloakAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("KEYCLOAK_PASSWORD", keycloakAdminSecret(cr), corev1.BasicAuthPasswordKey),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "imports",
							MountPath: "/etc/grafana/imports",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "grafana",
					Image: withDefault(cr.Spec.Grafana.Image, "docker.io/grafana/grafana:latest"),
					Command: []string{
						"sh", "-c", `
							export GF_AUTH_GENERIC_OAUTH_CLIENT_SECRET=$$(cat /etc/grafana/imports/clientsecret)
							/run.sh
						`,
					},
					Env: []corev1.EnvVar{
						{
							Name:  "GF_INSTALL_PLUGINS",
							Value: "simpod-json-datasource",
						},
						{
							Name:  "GF_SERVER_ROOT_URL",
							Value: url(cr.Spec.Grafana.Route, "must-set-grafana-route.io") + "/",
						},
						{
							Name:  "GF_USERS_DEFAULT_THEME",
							Value: withDefault(cr.Spec.Grafana.Theme, "light"),
						},
						{
							Name:  "GF_SECURITY_ALLOW_EMBEDDING",
							Value: "true",
						},
						{
							Name:  "GF_AUTH_DISABLE_LOGIN_FORM",
							Value: "true",
						},
						{
							Name:  "GF_AUTH_OAUTH_AUTO_LOGIN",
							Value: "true",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_ENABLED",
							Value: "true",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_CLIENT_ID",
							Value: "grafana",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_SCOPES",
							Value: "openid profile email",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_ALLOW_SIGN_UP",
							Value: "false",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_AUTH_URL",
							Value: url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io") + "/auth/realms/horreum/protocol/openid-connect/auth",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_TOKEN_URL",
							Value: keycloakURL + "/auth/realms/horreum/protocol/openid-connect/token",
						},
						{
							Name:  "GF_AUTH_GENERIC_OAUTH_API_URL",
							Value: keycloakURL + "/auth/realms/horreum/protocol/openid-connect/userinfo",
						},
						secretEnv("GF_SECURITY_ADMIN_USER", grafanaAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("GF_SECURITY_ADMIN_PASSWORD", grafanaAdminSecret(cr), corev1.BasicAuthPasswordKey),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "imports",
							MountPath: "/etc/grafana/imports",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "imports",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}
}

func grafanaService(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: int32(80),
					TargetPort: intstr.IntOrString{
						IntVal: 3000,
					},
				},
			},
			Selector: map[string]string{
				"app":     cr.Name,
				"service": "grafana",
			},
		},
	}
}

func grafanaRoute(cr *hyperfoilv1alpha1.Horreum, r *ReconcileHorreum) (*routev1.Route, error) {
	return route(cr.Spec.Grafana.Route, "-grafana", cr, r)
}
