package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func keycloakPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	initContainers := make([]corev1.Container, 0)
	initContainers = append(initContainers, corev1.Container{
		Name:            "copy-imports",
		Image:           appImage(cr),
		ImagePullPolicy: corev1.PullAlways,
		Command: []string{
			"sh", "-x", "-c", `cat /deployments/imports/keycloak-horreum.json ` +
				`| jq -r '.clients |= map(if .clientId | startswith("horreum") then ` +
				`(.rootUrl = "$(APP_URL)/") | (.adminUrl = "$(APP_URL)") | ` +
				`(.webOrigins = [ "$(APP_URL)" ]) | (.redirectUris = [ "$(APP_URL)/*"]) else . end)' ` +
				`| jq -r '.clients |= map(if .clientId | startswith("grafana") then ` +
				`(.rootUrl = "$(GRAFANA_URL)/") | (.adminUrl = "$(GRAFANA_URL)") | ` +
				`(.webOrigins = [ "$(GRAFANA_URL)" ]) | (.redirectUris = [ "$(GRAFANA_URL)/*"]) else . end)' ` +
				`> /etc/keycloak/imports/keycloak-horreum.json`,
		},
		Env: []corev1.EnvVar{
			{
				Name: "APP_URL",
				// TODO: this won't work without route set
				Value: url(cr.Spec.Route, "must-set-route.io"),
			},
			{
				Name:  "GRAFANA_URL",
				Value: url(cr.Spec.Grafana.Route, "must-set-route.io"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "imports",
				MountPath: "/etc/keycloak/imports",
			},
		},
	})
	volumes := []corev1.Volume{
		{
			Name: "imports",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "imports",
			MountPath: "/etc/keycloak/imports",
		},
	}
	if cr.Spec.Keycloak.Route.TLS != "" {
		// TODO: setup X509_CA_BUNDLE
		volumes = append(volumes, corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cr.Spec.Keycloak.Route.TLS,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/etc/x509/https",
		})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-keycloak",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":     cr.Name,
				"service": "keycloak",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: initContainers,
			Containers: []corev1.Container{
				{
					Name:  "keycloak",
					Image: withDefault(cr.Spec.Keycloak.Image, "quay.io/keycloak/keycloak:latest"),
					Args: []string{
						"-Dkeycloak.profile.feature.upload_scripts=enabled",
						"-Dkeycloak.migration.action=import",
						"-Dkeycloak.migration.provider=singleFile",
						"-Dkeycloak.migration.file=/etc/keycloak/imports/keycloak-horreum.json",
						"-Dkeycloak.migration.strategy=IGNORE_EXISTING",
					},
					Env: []corev1.EnvVar{
						secretEnv("KEYCLOAK_USER", keycloakAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("KEYCLOAK_PASSWORD", keycloakAdminSecret(cr), corev1.BasicAuthPasswordKey),
						{
							Name:  "DB_VENDOR",
							Value: "postgres",
						},
						{
							Name:  "DB_ADDR",
							Value: withDefault(cr.Spec.Keycloak.Database.Host, dbDefaultHost(cr)),
						},
						{
							Name:  "DB_PORT",
							Value: withDefaultInt(cr.Spec.Keycloak.Database.Port, dbDefaultPort(cr)),
						},
						{
							Name:  "DB_DATABASE",
							Value: withDefault(cr.Spec.Keycloak.Database.Name, "keycloak"),
						},
						secretEnv("DB_USER", keycloakDbSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("DB_PASSWORD", keycloakDbSecret(cr), corev1.BasicAuthPasswordKey),
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8080,
						},
					},
					VolumeMounts: volumeMounts,
				},
			},
			Volumes: volumes,
		},
	}
}

func keycloakService(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-keycloak",
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
				"service": "keycloak",
			},
		},
	}
}

func keycloakServiceSecure(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	// Openshift routes cannot select port
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-keycloak-secure",
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: int32(443),
					TargetPort: intstr.IntOrString{
						IntVal: 8443,
					},
				},
			},
			Selector: map[string]string{
				"app":     cr.Name,
				"service": "keycloak",
			},
		},
	}
}

func keycloakRoute(cr *hyperfoilv1alpha1.Horreum) *routev1.Route {
	subdomain := ""
	if cr.Spec.Keycloak.Route.Host == "" {
		subdomain = cr.Name + "-keycloak"
	}
	svc := cr.Name + "-keycloak-secure"
	tls := &routev1.TLSConfig{
		Termination: routev1.TLSTerminationPassthrough,
	}
	if cr.Spec.Keycloak.Route.TLS == "" {
		svc = cr.Name + "-keycloak"
		tls = nil
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-keycloak",
			Namespace: cr.Namespace,
		},
		Spec: routev1.RouteSpec{
			Host:      cr.Spec.Keycloak.Route.Host,
			Subdomain: subdomain,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: svc,
			},
			TLS: tls,
		},
	}
}
