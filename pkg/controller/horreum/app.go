package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func appPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	keycloakURL := keycloakURL(cr)
	grafanaHost := cr.Name + "-grafana." + cr.Namespace + ".svc"
	if cr.Spec.Grafana.ExternalHost != "" {
		grafanaHost = cr.Spec.Grafana.ExternalHost + ":" + cr.Spec.Grafana.ExternalPort
	}
	horreumEnv := []corev1.EnvVar{
		{
			Name:  "QUARKUS_DATASOURCE_JDBC_URL",
			Value: dbURL(cr, &cr.Spec.Database, "horreum"),
		},
		secretEnv("QUARKUS_DATASOURCE_USERNAME", appUserSecret(cr), corev1.BasicAuthUsernameKey),
		secretEnv("QUARKUS_DATASOURCE_PASSWORD", appUserSecret(cr), corev1.BasicAuthPasswordKey),
		{
			Name:  "QUARKUS_DATASOURCE_MIGRATION_JDBC_URL",
			Value: dbURL(cr, &cr.Spec.Database, "horreum"),
		},
		secretEnv("QUARKUS_DATASOURCE_MIGRATION_USERNAME", dbAdminSecret(cr), corev1.BasicAuthUsernameKey),
		secretEnv("QUARKUS_DATASOURCE_MIGRATION_PASSWORD", dbAdminSecret(cr), corev1.BasicAuthPasswordKey),
		secretEnv("HORREUM_DB_SECRET", appUserSecret(cr), "dbsecret"),
		{
			Name:  "QUARKUS_OIDC_AUTH_SERVER_URL",
			Value: keycloakURL + "/auth/realms/horreum",
		},
		{
			Name:  "QUARKUS_OIDC_TOKEN_ISSUER",
			Value: url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io") + "/auth/realms/horreum",
		},
		{
			Name:  "HORREUM_INTERNAL_URL",
			Value: "http://" + cr.Name + "." + cr.Namespace + ".svc",
		},
		{
			Name:  "HORREUM_KEYCLOAK_URL",
			Value: url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io") + "/auth",
		},
		{
			Name:  "HORREUM_GRAFANA_URL",
			Value: url(cr.Spec.Grafana.Route, "must-set-grafana-route.io"),
		},
		{
			Name:  "HORREUM_GRAFANA_MP_REST_URL",
			Value: "http://" + grafanaHost,
		},
		// This is needed because Grafana doesn't support API keys for user management
		secretEnv("HORREUM_GRAFANA_ADMIN_USER", grafanaAdminSecret(cr), corev1.BasicAuthUsernameKey),
		secretEnv("HORREUM_GRAFANA_ADMIN_PASSWORD", grafanaAdminSecret(cr), corev1.BasicAuthPasswordKey),
	}
	if javaOptions, ok := cr.ObjectMeta.Annotations["java-options"]; ok {
		horreumEnv = append(horreumEnv, corev1.EnvVar{
			Name:  "JAVA_OPTIONS",
			Value: javaOptions,
		})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-app",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":     cr.Name,
				"service": "app",
			},
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &[]int64{0}[0],
			InitContainers: []corev1.Container{
				{
					Name:            "set-imports",
					Image:           appImage(cr),
					ImagePullPolicy: corev1.PullAlways,
					Command: []string{
						"sh", "-x", "-c", `
							cp /deployments/imports/* /etc/horreum/imports
							export KC_URL='` + keycloakURL + `'
							export TOKEN=$$(curl -s $KC_URL/auth/realms/master/protocol/openid-connect/token -X POST -H 'content-type: application/x-www-form-urlencoded' -d 'username=$(KEYCLOAK_USER)&password=$(KEYCLOAK_PASSWORD)&grant_type=password&client_id=admin-cli' | jq -r .access_token)
							export CLIENTID=$$(curl -s $KC_URL/auth/admin/realms/horreum/clients -H 'Authorization: Bearer '$TOKEN | jq -r '.[] | select(.clientId=="horreum") | .id')
							export CLIENTSECRET=$$(curl -s $KC_URL/auth/admin/realms/horreum/clients/$CLIENTID/client-secret -X POST -H 'Authorization: Bearer '$TOKEN | jq -r '.value')
							[ -n "$CLIENTSECRET" ] || exit 1;
							echo $CLIENTSECRET > /etc/horreum/imports/clientsecret
						`,
					},
					Env: []corev1.EnvVar{
						secretEnv("KEYCLOAK_USER", keycloakAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("KEYCLOAK_PASSWORD", keycloakAdminSecret(cr), corev1.BasicAuthPasswordKey),
						secretEnv("GRAFANA_ADMIN_USER", grafanaAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("GRAFANA_ADMIN_PASSWORD", grafanaAdminSecret(cr), corev1.BasicAuthPasswordKey),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "imports",
							MountPath: "/etc/horreum/imports",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "horreum",
					Image: appImage(cr),
					Command: []string{
						"sh", "-c", `
							export QUARKUS_OIDC_CREDENTIALS_SECRET=$$(cat /etc/horreum/imports/clientsecret)
							export HORREUM_GRAFANA_API_KEY=$$(cat /etc/horreum/imports/grafana_api_key)
							/deployments/horreum.sh
						`,
					},
					Env: horreumEnv,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "imports",
							MountPath: "/etc/horreum/imports",
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

func appService(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: int32(80),
					TargetPort: intstr.IntOrString{
						IntVal: 8080,
					},
				},
			},
			Selector: map[string]string{
				"app":     cr.Name,
				"service": "app",
			},
		},
	}
}

func appRoute(cr *hyperfoilv1alpha1.Horreum, r *ReconcileHorreum) (*routev1.Route, error) {
	return route(cr.Spec.Route, "", cr, r)
}
