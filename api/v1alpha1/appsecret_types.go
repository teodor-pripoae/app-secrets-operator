/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AppSecretSpec defines the desired state of AppSecret
type AppSecretSpec struct {
	// Describe here environment variables.
	// Some of them can be simple strings, some of them can be references to other secrets.
	// Example:
	// env:
	//   DB_HOST: "localhost"
	//   DB_PORT: "5432"
	//   DB_USERNAME: "secret://my-secret/db-username"
	//   DB_PASSWORD: "secret://my-secret/db-password"
	Env map[string]string `json:"env,omitempty"`
}

// AppSecretStatus defines the observed state of AppSecret
type AppSecretStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AppSecret is the Schema for the appsecrets API
type AppSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppSecretSpec   `json:"spec,omitempty"`
	Status AppSecretStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppSecretList contains a list of AppSecret
type AppSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppSecret{}, &AppSecretList{})
}
