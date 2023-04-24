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
	"strconv"
	"time"

	"github.com/pkg/errors"
	secretsv1alpha1 "github.com/teodor-pripoae/app-secrets-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	secret := &v1.Secret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l.Info("Reconciling secret", "secret", secret.Name)

	annotation := dependsSecretAnnotation + "/" + secret.Name

	appSecrets := &secretsv1alpha1.AppSecretList{}
	options := []client.ListOption{
		client.InNamespace(req.Namespace),
	}
	err = r.List(ctx, appSecrets, options...)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to list secrets")
	}

	for _, s := range appSecrets.Items {
		if s.Annotations != nil && s.Annotations[annotation] == "true" {
			s.Annotations[lastTimeReconciled] = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
			if err := r.Update(ctx, &s); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to update app secret %s", s.Name)
			}
			l.Info("Marked app secret for reconcile", "appSecret", s.Name)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Secret{}).
		Complete(r)
}
