package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func appPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	keycloakURL := "http://" + cr.Name + "-keycloak." + cr.Namespace + ".svc"
	if cr.Spec.Keycloak.External {
		keycloakURL = url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io")
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
				corev1.Container{
					Name:            "set-imports",
					Image:           appImage(cr),
					ImagePullPolicy: corev1.PullAlways,
					Command: []string{
						"sh", "-x", "-c", `
							cp /deployments/imports/* /etc/horreum/imports
							export KC_URL='` + keycloakURL + `'
							export TOKEN=$$(curl -s $KC_URL/auth/realms/master/protocol/openid-connect/token -X POST -H 'content-type: application/x-www-form-urlencoded' -d 'username=$(KEYCLOAK_USER)&password=$(KEYCLOAK_PASSWORD)&grant_type=password&client_id=admin-cli' -s | jq -r .access_token)
							export CLIENTID=$$(curl -s $KC_URL/auth/admin/realms/hyperfoil/clients -H 'Authorization: Bearer '$TOKEN | jq -r '.[] | select(.clientId=="horreum") | .id')
							export CLIENTSECRET=$$(curl -s $KC_URL/auth/admin/realms/hyperfoil/clients/$CLIENTID/client-secret -H 'Authorization: Bearer '$TOKEN | jq -r '.value')
							[ -n "$CLIENTSECRET" ] || exit 1;
							echo $CLIENTSECRET > /etc/horreum/imports/clientsecret
						`,
					},
					Env: []corev1.EnvVar{
						secretEnv("KEYCLOAK_USER", keycloakAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("KEYCLOAK_PASSWORD", keycloakAdminSecret(cr), corev1.BasicAuthPasswordKey),
					},
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      "imports",
							MountPath: "/etc/horreum/imports",
						},
					},
				},
				corev1.Container{
					Name:  "init-db",
					Image: dbImage(cr),
					Command: []string{
						"bash", "-x", "-c", `
							psql -c "SELECT 1;" || exit 1 # fail if connection does not work
							if psql -t -c "SELECT 1 FROM pg_tables WHERE tablename = 'dbsecret'" | grep -q 1; then
								echo "Database structure seems in place."
							else
								psql -f /etc/horreum/imports/structure.sql
								psql -c "INSERT INTO dbsecret (passphrase) VALUES ('$(APP_DB_SECRET)');"
								psql -f /etc/horreum/imports/policies.sql
							fi
							if psql -t -c "SELECT 1 FROM pg_roles WHERE rolname = '$(APP_USER)';" | grep -q 1; then
								echo "Database role $(APP_USER) already exists.";
							else
								psql -c "CREATE ROLE \"$(APP_USER)\" noinherit login password '$(APP_PASSWORD)';"
							fi
							if [ $$(psql -t -c "SELECT count(*) FROM information_schema.role_table_grants WHERE grantee='$(APP_USER)';") == "0" ]; then
								psql -c "GRANT select, insert, delete, update ON ALL TABLES IN SCHEMA public TO $(APP_USER);"
								psql -c "REVOKE ALL ON dbsecret FROM $(APP_USER);"
								psql -c "GRANT ALL ON ALL sequences IN SCHEMA public TO $(APP_USER);"
							else
								echo "Role seems to already have some table grants."
							fi
						`,
					},
					Env: append(databaseAccessEnvVars(cr),
						secretEnv("APP_USER", appUserSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("APP_PASSWORD", appUserSecret(cr), corev1.BasicAuthPasswordKey),
						secretEnv("APP_DB_SECRET", appUserSecret(cr), "dbsecret"),
					),
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
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
							/deployments/run-java.sh
						`,
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "QUARKUS_DATASOURCE_URL",
							Value: dbURL(cr, &cr.Spec.Database, "horreum"),
						},
						secretEnv("QUARKUS_DATASOURCE_USERNAME", appUserSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("QUARKUS_DATASOURCE_PASSWORD", appUserSecret(cr), corev1.BasicAuthPasswordKey),
						secretEnv("REPO_DB_SECRET", appUserSecret(cr), "dbsecret"),
						corev1.EnvVar{
							Name:  "QUARKUS_OIDC_AUTH_SERVER_URL",
							Value: keycloakURL + "/auth/realms/hyperfoil",
						},
						corev1.EnvVar{
							Name:  "REPO_KEYCLOAK_URL",
							Value: url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io") + "/auth",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      "imports",
							MountPath: "/etc/horreum/imports",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				corev1.Volume{
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
				corev1.ServicePort{
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
