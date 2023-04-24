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

package controllers

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	secretsv1alpha1 "github.com/teodor-pripoae/app-secrets-operator/api/v1alpha1"
)

const (
	finalizer               = "secrets.toni.systems/finalizer"
	dependsSecretAnnotation = "secret-name.secrets.toni.systems"
	lastTimeReconciled      = "secrets.toni.systems/last-time-reconciled"
)

// AppSecretReconciler reconciles a AppSecret object
type AppSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=secrets.toni.systems,resources=appsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=secrets.toni.systems,resources=appsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=secrets.toni.systems,resources=appsecrets/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *AppSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	appSecret := &secretsv1alpha1.AppSecret{}
	err := r.Get(ctx, req.NamespacedName, appSecret)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !appSecret.ObjectMeta.DeletionTimestamp.IsZero() {
		// remove finalizer
		finalizers := []string{}
		for _, f := range appSecret.Finalizers {
			if f != finalizer {
				finalizers = append(finalizers, f)
			}
		}
		appSecret.Finalizers = finalizers
		l.Info("Removing finalizer", "appSecret", appSecret.Name)
		err = r.Update(ctx, appSecret)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to update appSecret %s", req.NamespacedName.Name)
		}

		return ctrl.Result{}, nil
	}

	l.Info("Reconciling AppSecret", "appSecret", appSecret.Name)

	secrets := getSecretNames(appSecret)
	secretValues, err := r.fetchSecrets(ctx, req.Namespace, secrets)

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to fetch secrets")
	}

	data, err := generateSecretData(appSecret, secretValues)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to generate secret data")
	}

	secret := &v1.Secret{}
	err = r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch secret")
		}

		secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appSecret.Name,
				Namespace: appSecret.Namespace,
				Labels:    appSecret.Labels,
			},
			StringData: data,
		}
		ctrl.SetControllerReference(appSecret, secret, r.Scheme)

		l.Info("Creating secret", "secret", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to create secret")
		}
	} else {
		secret.Labels = appSecret.Labels
		secret.StringData = data
		l.Info("Updating secret", "secret", secret.Name)
		err = r.Update(ctx, secret)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update secret")
		}
	}

	toBeUpdated := false

	found := false
	for _, f := range appSecret.Finalizers {
		if f == finalizer {
			found = true
			break
		}
	}
	if !found {
		appSecret.Finalizers = append(appSecret.Finalizers, finalizer)
		toBeUpdated = true
	}

	annotations := map[string]string{}
	for _, s := range secrets {
		annotations[dependsSecretAnnotation+"/"+s.Name] = "true"
	}
	if !annotationsEqual(secret.Annotations, annotations) {
		ann := map[string]string{}
		for k, v := range secret.Annotations {
			if !strings.HasPrefix(k, dependsSecretAnnotation) {
				ann[k] = v
			}
		}

		for k, v := range annotations {
			ann[k] = v
		}
		appSecret.Annotations = ann
		toBeUpdated = true
	}

	if toBeUpdated {
		err = r.Update(ctx, appSecret)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update appSecret")
		}
		l.Info("Updated appSecret", "appSecret", appSecret.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.AppSecret{}).
		Owns(&v1.Secret{}).
		Complete(r)
}

func (r *AppSecretReconciler) fetchSecrets(ctx context.Context, namespace string, secrets map[string]*SecretReference) (map[string]map[string]string, error) {
	secretValues := map[string]map[string]string{}

	for name, secret := range secrets {
		s := &v1.Secret{}
		err := r.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, s)

		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch secret %s", name)
		}

		secretValues[name] = map[string]string{}

		for key, _ := range secret.Keys {
			secretValues[name][key] = string(s.Data[key])
		}
	}

	return secretValues, nil
}

type SecretReference struct {
	Name string
	Keys map[string]bool
}

type ParsedValue struct {
	Name string
	Key  string
}

func getSecretNames(appSecret *secretsv1alpha1.AppSecret) map[string]*SecretReference {
	secrets := map[string]*SecretReference{}

	for _, value := range appSecret.Spec.Env {
		d, found := parseValue(value)
		if !found {
			continue
		}

		s, found := secrets[d.Name]
		if !found {
			s = &SecretReference{
				Name: d.Name,
				Keys: map[string]bool{},
			}
		}

		s.Keys[d.Key] = true
		secrets[d.Name] = s
	}

	return secrets
}

func generateSecretData(appSecret *secretsv1alpha1.AppSecret, secretValues map[string]map[string]string) (map[string]string, error) {
	data := map[string]string{}

	for key, value := range appSecret.Spec.Env {
		d, found := parseValue(value)
		if !found {
			data[key] = value
			continue
		}

		s, found := secretValues[d.Name]
		if !found {
			return nil, errors.Errorf("secret %s not found", d.Name)
		}

		v, found := s[d.Key]
		if !found {
			return nil, errors.Errorf("key %s not found in secret %s", d.Key, d.Name)
		}

		data[key] = v
	}

	return data, nil
}

func parseValue(value string) (*ParsedValue, bool) {
	if !strings.HasPrefix(value, "secret://") {
		return nil, false
	}

	parts := strings.Split(value, "/")
	if len(parts) != 4 {
		return nil, false
	}

	return &ParsedValue{
		Name: parts[2],
		Key:  parts[3],
	}, true
}

func annotationsEqual(a, b map[string]string) bool {
	for k, v := range b {
		if strings.HasPrefix(k, dependsSecretAnnotation) && a[k] != v {
			return false
		}
	}

	for k, v := range a {
		if strings.HasPrefix(k, dependsSecretAnnotation) && b[k] != v {
			return false
		}
	}

	return true
}
