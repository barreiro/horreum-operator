package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func appPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	keycloakURL := keycloakInternalURL(cr)
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
			// TODO: it's not possible to set up custom CA for OIDC
			// https://github.com/quarkusio/quarkus/issues/18002
			Name:  "QUARKUS_OIDC_TLS_VERIFICATION",
			Value: "none",
		},
		{
			Name:  "HORREUM_INTERNAL_URL",
			Value: innerProtocol(cr.Spec.Route) + cr.Name + "." + cr.Namespace + ".svc",
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
			Value: grafanaInternalURL(cr),
		},
		{
			Name:  "HORREUM_GRAFANA_MP_REST_TRUSTSTORE",
			Value: "/etc/horreum/imports/service-ca.keystore",
		},
		{
			Name:  "HORREUM_GRAFANA_MP_REST_TRUSTSTOREPASSWORD",
			Value: "password",
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
	volumes := []corev1.Volume{
		{
			Name: "imports",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "service-ca",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "service-ca.crt",
					},
				},
			},
		},
	}
	mounts := []corev1.VolumeMount{
		{
			Name:      "imports",
			MountPath: "/etc/horreum/imports",
		},
	}
	routeType := cr.Spec.Route.Type
	if routeType == "passthrough" || routeType == "reencrypt" || routeType == "" {
		secretName := cr.Name + "-app-certs"
		if cr.Spec.Route.Type == "passthrough" {
			secretName = cr.Spec.Route.TLS
		}
		volumes = append(volumes, corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/opt/certs",
		})
		horreumEnv = append(horreumEnv, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_FILE",
			Value: "/opt/certs/" + corev1.TLSCertKey,
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_KEY_FILE",
			Value: "/opt/certs/" + corev1.TLSPrivateKeyKey,
		})
	}
	caCertArg := ""
	if routeType == "reencrypt" || routeType == "" {
		caCertArg = "--cacert /etc/ssl/certs/service-ca.crt"
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
							export TOKEN=$$(curl -s ` + caCertArg + ` $KC_URL/auth/realms/master/protocol/openid-connect/token -X POST -H 'content-type: application/x-www-form-urlencoded' -d 'username=$(KEYCLOAK_USER)&password=$(KEYCLOAK_PASSWORD)&grant_type=password&client_id=admin-cli' | jq -r .access_token)
							USER_READER_PWD=$$(openssl rand -base64 32)
							curl -s $KC_URL/auth/admin/realms/horreum/users -H "Authorization: Bearer "$TOKEN -X POST -d '{"username":"__user_reader","enabled":true,"credentials":[{"type":"password","value":"'$USER_READER_PWD'"}],"email":"none@example.com"}' -H 'content-type: application/json'
                            USER_READER_ID=$(curl -s $KC_URL/auth/admin/realms/horreum/users -H "Authorization: Bearer "$TOKEN | jq -r '.[] | select(.username=="__user_reader").id')
							OFFLINE_ACCESS_ID=$(curl -s $KC_URL/auth/admin/realms/horreum/roles/offline_access -H "Authorization: Bearer "$TOKEN | jq -r '.id')
							curl -s $KC_URL/auth/admin/realms/horreum/users/$USER_READER_ID/role-mappings/realm -H "Authorization: Bearer "$TOKEN -H 'content-type: application/json' -X POST -d '[{"id":"'$OFFLINE_ACCESS_ID'","name":"offline_access"}]'
                            REALM_MANAGEMENT_CLIENTID=$(curl -s $KC_URL/auth/admin/realms/horreum/clients -H "Authorization: Bearer "$TOKEN | jq -r '.[] | select(.clientId=="realm-management").id')
                            VIEW_USERS_ID=$(curl -s $KC_URL/auth/admin/realms/horreum/clients/$REALM_MANAGEMENT_CLIENTID/roles/view-users -H "Authorization: Bearer "$TOKEN | jq -r '.id')
							curl -s $KC_URL/auth/admin/realms/horreum/users/$USER_READER_ID/role-mappings/clients/$REALM_MANAGEMENT_CLIENTID -H "Authorization: Bearer "$TOKEN -H 'content-type: application/json' -X POST -d '[{"id":"'$VIEW_USERS_ID'","name":"view-users"}]'
							USER_READER_TOKEN=$(curl -s -X POST $KC_URL/auth/realms/horreum/protocol/openid-connect/token -H 'content-type: application/x-www-form-urlencoded' -d 'username=__user_reader&password='$USER_READER_PWD'&grant_type=password&client_id=horreum-ui&scope=offline_access' | jq -r .refresh_token)
							echo $USER_READER_TOKEN >> /etc/horreum/imports/user_reader_token

							export CLIENTID=$$(curl -s ` + caCertArg + ` $KC_URL/auth/admin/realms/horreum/clients -H 'Authorization: Bearer '$TOKEN | jq -r '.[] | select(.clientId=="horreum") | .id')
							export CLIENTSECRET=$$(curl -s ` + caCertArg + ` $KC_URL/auth/admin/realms/horreum/clients/$CLIENTID/client-secret -X POST -H 'Authorization: Bearer '$TOKEN | jq -r '.value')
							[ -n "$CLIENTSECRET" ] || exit 1;
							echo $CLIENTSECRET > /etc/horreum/imports/clientsecret
							keytool -keystore /etc/horreum/imports/service-ca.keystore -storepass password -noprompt -trustcacerts -import -alias service-ca -file /etc/ssl/certs/service-ca.crt
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
						{
							Name:      "service-ca",
							MountPath: "/etc/ssl/certs/service-ca.crt",
							SubPath:   "service-ca.crt",
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
							export HORREUM_KEYCLOAK_USER_READER_TOKEN=$$(cat /etc/horreum/imports/user_reader_token)
							/deployments/horreum.sh
						`,
					},
					Env:          horreumEnv,
					VolumeMounts: mounts,
				},
			},
			Volumes: volumes,
		},
	}
}

func appService(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": cr.Name + "-app-certs",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				servicePort(cr.Spec.Route, 8080, 8443),
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
