package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func reportPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	var reportsVolumeSource = corev1.VolumeSource{}
	if cr.Spec.Report.PersistentVolumeClaim != "" {
		reportsVolumeSource.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: cr.Spec.Report.PersistentVolumeClaim,
		}
	} else {
		reportsVolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
	}
	host := "http://" + cr.Name + "-report." + cr.Namespace + ".svc"
	hookURL := host + "/cgi-bin/convert.sh?prefix=${$.id}-&subpath=.data"
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-report",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":     cr.Name,
				"service": "report",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				corev1.Container{
					Name:  "add-hook",
					Image: dbImage(cr),
					Command: []string{
						"bash", "-x", "-c", `
							if psql -t -c "SELECT 1 FROM hook WHERE url like '` + host + `%';" | grep -q 1; then
								echo "Hook already installed.";
							else
								psql -c "INSERT INTO hook(id, type, url, target, active) VALUES(nextval('hook_id_seq'), 'new/run', '"'` + hookURL + `'"', -1, true);"
							fi
						`,
					},
					Env: databaseAccessEnvVars(cr),
				},
			},
			Containers: []corev1.Container{
				corev1.Container{
					Name:            "report",
					Image:           withDefault(cr.Spec.Report.Image, "quay.io/hyperfoil/hyperfoil-report:latest"),
					ImagePullPolicy: corev1.PullAlways,
					Ports: []corev1.ContainerPort{
						corev1.ContainerPort{
							Name:          "http",
							ContainerPort: 8080,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      "reports",
							MountPath: "/var/www/localhost/htdocs",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				corev1.Volume{
					Name:         "reports",
					VolumeSource: reportsVolumeSource,
				},
			},
		},
	}
}

func reportService(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-report",
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
				"service": "report",
			},
		},
	}
}

func reportRoute(cr *hyperfoilv1alpha1.Horreum) *routev1.Route {
	subdomain := ""
	if cr.Spec.Keycloak.Route == "" {
		subdomain = cr.Name + "-report"
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-report",
			Namespace: cr.Namespace,
		},
		Spec: routev1.RouteSpec{
			Host:      cr.Spec.Report.Route,
			Subdomain: subdomain,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: cr.Name + "-report",
			},
		},
	}
}
