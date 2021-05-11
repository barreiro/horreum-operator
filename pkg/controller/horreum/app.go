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
							GRAFANA_ADMIN_URL=http://$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD@` + grafanaHost + `
							for DATASOURCE_ID in $(curl $GRAFANA_ADMIN_URL/api/datasources | jq .[].id); do curl -s -X DELETE $GRAFANA_ADMIN_URL/api/datasources/$DATASOURCE_ID; done;
							while ! curl -s $GRAFANA_ADMIN_URL/api/datasources -H 'content-type: application/json' -d '{"name":"Horreum","type":"simpod-json-datasource","access":"proxy","url":"http://` + cr.Name + "." + cr.Namespace + `.svc/api/grafana","basicAuth":false,"withCredentials":false,"isDefault":true,"jsonData":{"oauthPassThru":true},"readOnly":false}'; do sleep 5; done;
							for KEY_ID in $(curl -s $GRAFANA_ADMIN_URL/api/auth/keys | jq .[].id); do curl -s $GRAFANA_ADMIN_URL/api/auth/keys/$KEY_ID -X DELETE; done
							GRAFANA_API_KEY=$$(curl -s $GRAFANA_ADMIN_URL/api/auth/keys -H 'content-type: application/json' -d '{"name":"Horreum","role":"Editor"}' | jq -r .key)
							[ -n "$GRAFANA_API_KEY" -a "$GRAFANA_API_KEY" != "null" ] || exit 1
							echo $GRAFANA_API_KEY >> /etc/horreum/imports/grafana_api_key
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
				{
					Name:  "init-db",
					Image: dbImage(cr),
					Command: []string{
						"bash", "-x", "-c", `
							psql -c "SELECT 1;" || exit 1 # fail if connection does not work
							if psql -t -c "SELECT 1 FROM pg_roles WHERE rolname = '$(APP_USER)';" | grep -q 1; then
								echo "Database role $(APP_USER) already exists.";
							else
								psql -c "CREATE ROLE \"$(APP_USER)\" noinherit login password '$(APP_PASSWORD)';"
							fi
							if [ $$(psql -t -c "SELECT count(*) FROM information_schema.role_table_grants WHERE grantee='$(APP_USER)';") == "0" ]; then
								psql -c "GRANT select, insert, delete, update ON ALL TABLES IN SCHEMA public TO \"$(APP_USER)\";"
								psql -c "REVOKE ALL ON dbsecret FROM \"$(APP_USER)\";"
								psql -c "GRANT ALL ON ALL sequences IN SCHEMA public TO \"$(APP_USER)\";"
							else
								echo "Role seems to already have some table grants."
							fi
						`,
					},
					Env: append(databaseAccessEnvVars(cr, &cr.Spec.Database),
						secretEnv("APP_USER", appUserSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("APP_PASSWORD", appUserSecret(cr), corev1.BasicAuthPasswordKey),
						secretEnv("APP_DB_SECRET", appUserSecret(cr), "dbsecret"),
					),
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
					Env: []corev1.EnvVar{
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
							Name:  "HORREUM_KEYCLOAK_URL",
							Value: url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io") + "/auth",
						},
					},
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
