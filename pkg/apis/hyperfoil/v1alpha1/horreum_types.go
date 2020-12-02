package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseSpec defines access info for a database
type DatabaseSpec struct {
	// Hostname for the database
	Host string `json:"host,omitempty"`
	// Database port; defaults to 5432
	Port int32 `json:"port,omitempty"`
	// Name of the database
	Name string `json:"name,omitempty"`
	// Name of secret resource with data `username` and `password`. Created if does not exist.
	Secret string `json:"secret,omitempty"`
}

// RouteSpec defines the route
type RouteSpec struct {
	// Hostname for external access
	Host string `json:"host,omitempty"`
	// Optional; Name of the secret hosting `tls.crt`, `tls.key` and optionally `ca.crt`
	TLS string `json:"tls,omitempty"`
}

// KeycloakSpec defines Keycloak setup
type KeycloakSpec struct {
	// Set to true if the Keycloak instance should not be deployed
	External bool `json:"external,omitempty"`
	// Image that should be used for Keycloak deployment. Defaults to docker.io/jboss/keycloak:latest
	Image string `json:"image,omitempty"`
	// Route for external access to the Keycloak instance.
	// When `external` is set to true, this will be used for internal access as well.
	Route RouteSpec `json:"route,omitempty"`
	// Secret used for admin access to the deployed Keycloak instance. Created if does not exist.
	// Must contain keys `username` and `password`.
	AdminSecret string `json:"adminSecret,omitempty"`
	// Database coordinates Keycloak should use
	Database DatabaseSpec `json:"database,omitempty"`
}

// PostgresSpec defines PostgreSQL database setup
type PostgresSpec struct {
	// Hostname of the external database. If empty, database will be deployed by this operator.
	ExternalHost string `json:"externalHost,omitempty"`
	// Port of the external database. Defaults to 5432.
	ExternalPort int32 `json:"externalPort,omitempty"`
	// Image used for PostgreSQL deployment. Defaults to docker.io/postgres:12
	Image string `json:"image,omitempty"`
	// Secret used for unrestricted access to the database. Created if does not exist.
	// Must contain keys `username` and `password`.
	AdminSecret string `json:"adminSecret,omitempty"`
	// Name of PVC where the database will store the data. If empty, ephemeral storage will be used.
	PersistentVolumeClaim string `json:"persistentVolumeClaim,omitempty"`
}

// ReportSpec defines hyperfoil-report pod setup
type ReportSpec struct {
	// True (or omitted) to deploy report pod.
	Enabled *bool `json:"enabled,omitempty"`
	// Image of the report tool. Defaults to quay.io/hyperfoil/hyperfoil-report:latest
	Image string `json:"image,omitempty"`
	// Route for external access.
	Route RouteSpec `json:"route,omitempty"`
}

// GrafanaSpec defines Grafana setup
type GrafanaSpec struct {
	// Hostname of the external Grafana. If empty, an instance of Grafana will be deployed
	ExternalHost string `json:"externalHost,omitempty"`
	// Port of the external Grafana. Defaults to 3000.
	ExternalPort string `json:"externalPort,omitempty"`
	// Secret used for admin access to Grafana. Created if it does not exist.
	// Must contain keys `username` and `password`.
	AdminSecret string `json:"adminSecret,omitempty"`
	// Custom Grafana image. Defaults to docker.io/grafana/grafana:latest
	Image string `json:"image,omitempty"`
	// Default theme that should be used - one of `dark` or `light`. Defaults to `light`.
	Theme string `json:"theme,omitempty"`
	// Route for external access.
	Route RouteSpec `json:"route,omitempty"`
}

// HorreumSpec defines the desired state of Horreum
type HorreumSpec struct {
	// Route for external access
	Route RouteSpec `json:"route,omitempty"`
	// Horreum image. Defaults to quay.io/hyperfoil/horreum:latest
	Image string `json:"image,omitempty"`
	// Database coordinates for Horreum data. Besides `username` and `password` the secret must
	// also contain key `dbsecret` that will be used to sign access to the database.
	Database DatabaseSpec `json:"database,omitempty"`
	// Keycloak specification
	Keycloak KeycloakSpec `json:"keycloak,omitempty"`
	// PostgreSQL specification
	Postgres PostgresSpec `json:"postgres,omitempty"`
	// Hyperfoil report tool specification
	Report ReportSpec `json:"report,omitempty"`
	// Grafana specification
	Grafana GrafanaSpec `json:"grafana,omitempty"`
}

// HorreumStatus defines the observed state of Horreum
type HorreumStatus struct {
	// Ready, Pending or Error.
	Status string `json:"status,omitempty"`
	// Last time state has changed.
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
	// Explanation for the current status.
	Reason string `json:"reason,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Horreum is the object configuring Horreum performance results repository
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=horreums,scope=Namespaced
// +kubebuilder:categories=all,hyperfoil
// +kubebuilder:resource:shortName=hrm
// +kubebuilder:printcolumn:name="Route",type="string",JSONPath=".spec.route",description="Horreum route"
// +kubebuilder:printcolumn:name="Keycloak Route",type="string",JSONPath=".spec.keycloak.route",description="Keycloak route"
// +kubebuilder:printcolumn:name="Hyperfoil Report Route",type="string",JSONPath=".spec.report.route",description="Hyperfoil Report route"
// +kubebuilder:printcolumn:name="Grafana Route",type="string",JSONPath=".spec.grafana.route",description="Grafana route"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status",description="Overall status"
type Horreum struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HorreumSpec   `json:"spec,omitempty"`
	Status HorreumStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HorreumList contains a list of Horreum
type HorreumList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Horreum `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Horreum{}, &HorreumList{})
}
