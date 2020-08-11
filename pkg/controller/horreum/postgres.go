package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func postgresPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
	labels := map[string]string{
		"app":     cr.Name,
		"service": "db",
	}

	dbVolumeSrc := corev1.VolumeSource{}
	if cr.Spec.Postgres.PersistentVolumeClaim != "" {
		dbVolumeSrc = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: cr.Spec.Postgres.PersistentVolumeClaim,
			},
		}
	} else {
		dbVolumeSrc = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-db",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "postgres",
					Image: dbImage(cr),
					Env: []corev1.EnvVar{
						{
							Name:  "POSTGRES_DB",
							Value: withDefault(cr.Spec.Database.Name, "horreum"),
						},
						secretEnv("POSTGRES_USER", dbAdminSecret(cr), corev1.BasicAuthUsernameKey),
						secretEnv("POSTGRES_PASSWORD", dbAdminSecret(cr), corev1.BasicAuthPasswordKey),
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "postgres",
							ContainerPort: 5432,
						},
					},
					SecurityContext: &corev1.SecurityContext{
						// Run with postgres UID - the same should be set on the PVC
						RunAsUser: &[]int64{999}[0],
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "db-volume",
							MountPath: "/var/lib/postgresql/data",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name:         "db-volume",
					VolumeSource: dbVolumeSrc,
				},
			},
		},
	}
}

func postgresService(cr *hyperfoilv1alpha1.Horreum) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-db",
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "postgres",
					Port: int32(5432),
					TargetPort: intstr.IntOrString{
						IntVal: 5432,
					},
				},
			},
			Selector: map[string]string{
				"app":     cr.Name,
				"service": "db",
			},
		},
	}
}
