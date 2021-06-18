package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
)

func dbDefaultHost(cr *hyperfoilv1alpha1.Horreum) string {
	return cr.Name + "-db." + cr.Namespace + ".svc"
}

func dbURL(cr *hyperfoilv1alpha1.Horreum, db *hyperfoilv1alpha1.DatabaseSpec, defName string) string {
	return "jdbc:postgresql://" + withDefault(db.Host, dbDefaultHost(cr)) +
		":" + withDefaultInt(db.Port, 5432) + "/" + withDefault(db.Name, defName)
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
	return withDefault(cr.Spec.Postgres.Image, "registry.redhat.io/rhel8/postgresql-12:latest")
}

func appImage(cr *hyperfoilv1alpha1.Horreum) string {
	return withDefault(cr.Spec.Image, "quay.io/hyperfoil/horreum:latest")
}

func keycloakInternalURL(cr *hyperfoilv1alpha1.Horreum) string {
	if cr.Spec.Keycloak.External.InternalUri != "" {
		return cr.Spec.Keycloak.External.InternalUri
	}
	if cr.Spec.Keycloak.External.PublicUri != "" {
		return cr.Spec.Keycloak.External.PublicUri
	}
	return innerProtocol(cr.Spec.Keycloak.Route) + cr.Name + "-keycloak." + cr.Namespace + ".svc"
}

func grafanaInternalURL(cr *hyperfoilv1alpha1.Horreum) string {
	if cr.Spec.Grafana.External.InternalUri != "" {
		return cr.Spec.Grafana.External.InternalUri
	}
	if cr.Spec.Grafana.External.PublicUri != "" {
		return cr.Spec.Grafana.External.PublicUri
	}
	return innerProtocol(cr.Spec.Grafana.Route) + cr.Name + "-grafana." + cr.Namespace + ".svc"
}
