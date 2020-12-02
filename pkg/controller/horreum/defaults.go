package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func dbDefaultHost(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Postgres.ExternalHost, cr.Name+"-db."+cr.Namespace+".svc")
}

func dbDefaultPort(cr *hyperfoilv1alpha1.Horreum) int32 {
	dbDefaultPort := cr.Spec.Postgres.ExternalPort
	if dbDefaultPort == 0 {
		dbDefaultPort = 5432
	}
	return dbDefaultPort
}

func dbURL(cr *hyperfoilv1alpha1.Horreum, db *hyperfoilv1alpha1.DatabaseSpec, defName string) string {
	return "jdbc:postgresql://" + withDefault(db.Host, dbDefaultHost(cr)) +
		":" + withDefaultInt(db.Port, dbDefaultPort(cr)) + "/" + withDefault(db.Name, defName)
}

func dbAdminSecret(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Postgres.AdminSecret, cr.Name+"-db-admin")
}

func appUserSecret(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Database.Secret, cr.Name+"-app")
}

func keycloakDbSecret(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Keycloak.Database.Secret, cr.Name+"-keycloak-db")
}

func keycloakAdminSecret(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Keycloak.AdminSecret, cr.Name+"-keycloak-admin")
}

func grafanaAdminSecret(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Grafana.AdminSecret, cr.Name+"-grafana-admin")
}

func dbImage(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Postgres.Image, "docker.io/postgres:12")
}

func appImage(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Image, "quay.io/hyperfoil/horreum:latest")
}

func databaseAccessEnvVars(cr *hyperfoilv1alpha1.Horreum, db *hyperfoilv1alpha1.DatabaseSpec) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "PGHOST",
			Value: withDefault(db.Host, dbDefaultHost(cr)),
		},
		{
			Name:  "PGPORT",
			Value: withDefaultInt(db.Port, dbDefaultPort(cr)),
		},
		{
			Name:  "PGDATABASE",
			Value: withDefault(cr.Spec.Database.Name, "horreum"),
		},
		secretEnv("PGUSER", dbAdminSecret(cr), corev1.BasicAuthUsernameKey),
		secretEnv("PGPASSWORD", dbAdminSecret(cr), corev1.BasicAuthPasswordKey),
	}
}

func keycloakURL(cr *hyperfoilv1alpha1.Horreum) string {
	if cr.Spec.Keycloak.External {
		return url(cr.Spec.Keycloak.Route, "must-set-keycloak-route.io")
	}
	return "http://" + cr.Name + "-keycloak." + cr.Namespace + ".svc"
}
