package horreum

import (
	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/pkg/apis/hyperfoil/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func reportPod(cr *hyperfoilv1alpha1.Horreum) *corev1.Pod {
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

func reportRoute(cr *hyperfoilv1alpha1.Horreum, r *ReconcileHorreum) (*routev1.Route, error) {
	return route(cr.Spec.Report.Route, "-report", cr, r)
}
