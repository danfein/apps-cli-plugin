/*
Copyright 2021 VMware, Inc.

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

package commands_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	diecorev1 "dies.dev/apis/core/v1"
	diemetav1 "dies.dev/apis/meta/v1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/apps-cli-plugin/pkg/apis"
	cartov1alpha1 "github.com/vmware-tanzu/apps-cli-plugin/pkg/apis/cartographer/v1alpha1"
	cli "github.com/vmware-tanzu/apps-cli-plugin/pkg/cli-runtime"
	"github.com/vmware-tanzu/apps-cli-plugin/pkg/cli-runtime/logs"
	clitesting "github.com/vmware-tanzu/apps-cli-plugin/pkg/cli-runtime/testing"
	"github.com/vmware-tanzu/apps-cli-plugin/pkg/cli-runtime/validation"
	watchhelper "github.com/vmware-tanzu/apps-cli-plugin/pkg/cli-runtime/watch"
	watchfakes "github.com/vmware-tanzu/apps-cli-plugin/pkg/cli-runtime/watch/fake"
	"github.com/vmware-tanzu/apps-cli-plugin/pkg/commands"
	diecartov1alpha1 "github.com/vmware-tanzu/apps-cli-plugin/pkg/dies/cartographer/v1alpha1"
	"github.com/vmware-tanzu/apps-cli-plugin/pkg/flags"
)

func TestWorkloadUpdateOptionsValidate(t *testing.T) {
	table := clitesting.ValidatableTestSuite{
		{
			Name: "valid options",
			Validatable: &commands.WorkloadUpdateOptions{
				WorkloadOptions: commands.WorkloadOptions{
					Namespace: "default",
					Name:      "my-resource",
					Env:       []string{"FOO=bar"},
					BuildEnv:  []string{"BAR=baz"},
				},
			},
			ShouldValidate: true,
		},
		{
			Name: "invalid options",
			Validatable: &commands.WorkloadUpdateOptions{
				WorkloadOptions: commands.WorkloadOptions{
					Namespace: "default",
					Name:      "my-resource",
					Env:       []string{"FOO"},
				},
			},
			ExpectFieldErrors: validation.ErrInvalidArrayValue("FOO", flags.EnvFlagName, 0),
		},
		{
			Name: "invalid build env options",
			Validatable: &commands.WorkloadUpdateOptions{
				WorkloadOptions: commands.WorkloadOptions{
					Namespace: "default",
					Name:      "my-resource",
					BuildEnv:  []string{"FOO"},
				},
			},
			ExpectFieldErrors: validation.ErrInvalidArrayValue("FOO", flags.BuildEnvFlagName, 0),
		},
	}

	table.Run(t)
}

func TestWorkloadUpdateCommand(t *testing.T) {
	defaultNamespace := "default"
	workloadName := "my-workload"

	scheme := runtime.NewScheme()
	_ = cartov1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	var cmd *cobra.Command

	parent := diecartov1alpha1.WorkloadBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Name(workloadName)
			d.Namespace(defaultNamespace)
		})
	sprintPetclinicWorkload := diecartov1alpha1.WorkloadBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Name("spring-petclinic")
			d.Namespace(defaultNamespace)
		})
	table := clitesting.CommandTestSuite{
		{
			Name:        "invalid args",
			Args:        []string{},
			ShouldError: true,
		},
		{
			Name: "noop",
			Args: []string{workloadName},
			GivenObjects: []client.Object{
				parent.SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
					d.Image("ubuntu:bionic")
				},
				),
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Workload is unchanged, skipping update
`,
		},
		{
			Name: "no source",
			Args: []string{workloadName},
			GivenObjects: []client.Object{
				parent,
			},
		},
		{
			Name: "not found",
			Args: []string{workloadName},
			GivenObjects: []client.Object{
				diecorev1.NamespaceBlank.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name(defaultNamespace)
					}),
				diecartov1alpha1.WorkloadBlank.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("foo")
					}),
			},
			WithReactors: []clitesting.ReactionFunc{
				clitesting.InduceFailure("get", "Workload", clitesting.InduceFailureOpts{
					Error: apierrors.NewNotFound(cartov1alpha1.Resource("Workload"), workloadName),
				}),
			},
			ShouldError: true,
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Workload "default/my-workload" not found
`,
		},
		{
			Name: "namespace not found",
			Args: []string{workloadName, flags.NamespaceFlagName, "foo"},
			WithReactors: []clitesting.ReactionFunc{
				clitesting.InduceFailure("get", "Namespace", clitesting.InduceFailureOpts{
					Error: apierrors.NewNotFound(corev1.Resource("Namespace"), "foo"),
				}),
			},
			ShouldError: true,
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Error: namespace "foo" not found, it may not exist or user does not have permissions to read it.
`,
		},
		{
			Name: "get failed",
			Args: []string{workloadName},
			WithReactors: []clitesting.ReactionFunc{
				clitesting.InduceFailure("get", "Workload"),
			},
			ShouldError: true,
		},
		{
			Name: "dry run",
			Args: []string{workloadName, flags.DryRunFlagName, flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

---
apiVersion: carto.run/v1alpha1
kind: Workload
metadata:
  creationTimestamp: "1970-01-01T00:00:01Z"
  name: my-workload
  namespace: default
  resourceVersion: "999"
spec:
  image: ubuntu:bionic
status:
  supplyChainRef: {}
`,
		},
		{
			Name: "error during update",
			Args: []string{workloadName, flags.DebugFlagName, flags.YesFlagName},
			WithReactors: []clitesting.ReactionFunc{
				clitesting.InduceFailure("update", "Workload"),
			},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						Params: []cartov1alpha1.Param{
							{
								Name:  "debug",
								Value: apiextensionsv1.JSON{Raw: []byte(`"true"`)},
							},
						},
					},
				},
			},
			ShouldError: true,
		},
		{
			Name: "update subPath for git source",
			Args: []string{workloadName, flags.SubPathFlagName, "./app", flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Source(
							&cartov1alpha1.Source{
								Git: &cartov1alpha1.GitSource{
									URL: "https://github.com/spring-projects/spring-petclinic.git",
									Ref: cartov1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						)
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
							Subpath: "./app",
						},
					},
				},
			},
		},
		{
			Name: "override subPath for source image source",
			Args: []string{workloadName, flags.SubPathFlagName, "./app", flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Source(
							&cartov1alpha1.Source{
								Image:   "ubuntu:source",
								Subpath: "./cmd",
							},
						)
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Image:   "ubuntu:source",
							Subpath: "./app",
						},
					},
				},
			},
		},
		{
			Name: "unset subPath for git source",
			Args: []string{workloadName, flags.SubPathFlagName, "./app", flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Source(
							&cartov1alpha1.Source{
								Git: &cartov1alpha1.GitSource{
									URL: "https://github.com/spring-projects/spring-petclinic.git",
									Ref: cartov1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						)
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
							Subpath: "./app",
						},
					},
				},
			},
		},
		{
			Name: "conflict during update",
			Args: []string{workloadName, flags.DebugFlagName, flags.YesFlagName},
			WithReactors: []clitesting.ReactionFunc{
				clitesting.InduceFailure("update", "Workload", clitesting.InduceFailureOpts{
					Error: apierrors.NewConflict(schema.GroupResource{Group: "carto.run", Resource: "workloads"}, workloadName, fmt.Errorf("induced conflict")),
				}),
			},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						Params: []cartov1alpha1.Param{
							{
								Name:  "debug",
								Value: apiextensionsv1.JSON{Raw: []byte(`"true"`)},
							},
						},
					},
				},
			},
			ShouldError: true,
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7,  7   |spec:
  8,  8   |  image: ubuntu:bionic
      9 + |  params:
     10 + |  - name: debug
     11 + |    value: "true"

Error: conflict updating workload, the object was modified by another user; please run the update command again
`,
		},
		{
			Name: "wait error with timeout",
			Args: []string{workloadName, flags.ServiceRefFlagName, "database=services.tanzu.vmware.com/v1alpha1:PostgreSQL:my-prod-db", flags.WaitFlagName, flags.YesFlagName, flags.WaitTimeoutFlagName, "1ns"},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			Prepare: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) (context.Context, error) {
				workload := &cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Status: cartov1alpha1.WorkloadStatus{
						Conditions: []metav1.Condition{
							{
								Type:   cartov1alpha1.WorkloadConditionReady,
								Status: metav1.ConditionTrue,
							},
						},
					},
				}
				fakeWatcher := watchfakes.NewFakeWithWatch(false, config.Client, []watch.Event{
					{Type: watch.Modified, Object: workload},
				})
				ctx = watchhelper.WithWatcher(ctx, fakeWatcher)
				return ctx, nil
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						ServiceClaims: []cartov1alpha1.WorkloadServiceClaim{
							{
								Name: "database",
								Ref: &cartov1alpha1.WorkloadServiceClaimReference{
									APIVersion: "services.tanzu.vmware.com/v1alpha1",
									Kind:       "PostgreSQL",
									Name:       "my-prod-db",
								},
							},
						},
					},
				},
			},
			ShouldError: true,
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7,  7   |spec:
  8,  8   |  image: ubuntu:bionic
      9 + |  serviceClaims:
     10 + |  - name: database
     11 + |    ref:
     12 + |      apiVersion: services.tanzu.vmware.com/v1alpha1
     13 + |      kind: PostgreSQL
     14 + |      name: my-prod-db

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload"
To get status: "tanzu apps workload get my-workload"

Waiting for workload "my-workload" to become ready...
Error: timeout after 1ns waiting for "my-workload" to become ready
`,
		},
		{
			Name: "wait error for false condition",
			Args: []string{workloadName, flags.ServiceRefFlagName, "database=services.tanzu.vmware.com/v1alpha1:PostgreSQL:my-prod-db", flags.WaitFlagName, flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			Prepare: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) (context.Context, error) {
				workload := &cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Status: cartov1alpha1.WorkloadStatus{
						Conditions: []metav1.Condition{
							{
								Type:    cartov1alpha1.WorkloadConditionReady,
								Status:  metav1.ConditionFalse,
								Reason:  "OopsieDoodle",
								Message: "a hopefully informative message about what went wrong",
							},
						},
					},
				}
				fakeWatcher := watchfakes.NewFakeWithWatch(false, config.Client, []watch.Event{
					{Type: watch.Modified, Object: workload},
				})
				ctx = watchhelper.WithWatcher(ctx, fakeWatcher)
				return ctx, nil
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						ServiceClaims: []cartov1alpha1.WorkloadServiceClaim{
							{
								Name: "database",
								Ref: &cartov1alpha1.WorkloadServiceClaimReference{
									APIVersion: "services.tanzu.vmware.com/v1alpha1",
									Kind:       "PostgreSQL",
									Name:       "my-prod-db",
								},
							},
						},
					},
				},
			},
			ShouldError: true,
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7,  7   |spec:
  8,  8   |  image: ubuntu:bionic
      9 + |  serviceClaims:
     10 + |  - name: database
     11 + |    ref:
     12 + |      apiVersion: services.tanzu.vmware.com/v1alpha1
     13 + |      kind: PostgreSQL
     14 + |      name: my-prod-db

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload"
To get status: "tanzu apps workload get my-workload"

Waiting for workload "my-workload" to become ready...
Error: Failed to become ready: a hopefully informative message about what went wrong
`,
		},
		{
			Name: "successful wait for ready condition",
			Args: []string{workloadName, flags.ServiceRefFlagName, "database=services.tanzu.vmware.com/v1alpha1:PostgreSQL:my-prod-db", flags.WaitFlagName, flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			Prepare: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) (context.Context, error) {
				workload := &cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Status: cartov1alpha1.WorkloadStatus{
						Conditions: []metav1.Condition{
							{
								Type:   cartov1alpha1.WorkloadConditionReady,
								Status: metav1.ConditionTrue,
							},
						},
					},
				}
				fakeWatcher := watchfakes.NewFakeWithWatch(false, config.Client, []watch.Event{
					{Type: watch.Modified, Object: workload},
				})
				ctx = watchhelper.WithWatcher(ctx, fakeWatcher)
				return ctx, nil
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						ServiceClaims: []cartov1alpha1.WorkloadServiceClaim{
							{
								Name: "database",
								Ref: &cartov1alpha1.WorkloadServiceClaimReference{
									APIVersion: "services.tanzu.vmware.com/v1alpha1",
									Kind:       "PostgreSQL",
									Name:       "my-prod-db",
								},
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7,  7   |spec:
  8,  8   |  image: ubuntu:bionic
      9 + |  serviceClaims:
     10 + |  - name: database
     11 + |    ref:
     12 + |      apiVersion: services.tanzu.vmware.com/v1alpha1
     13 + |      kind: PostgreSQL
     14 + |      name: my-prod-db

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload"
To get status: "tanzu apps workload get my-workload"

Waiting for workload "my-workload" to become ready...
Workload "my-workload" is ready
`,
		},
		{
			Name: "tail while waiting for ready condition",
			Args: []string{workloadName, flags.ServiceRefFlagName, "database=services.tanzu.vmware.com/v1alpha1:PostgreSQL:my-prod-db", flags.YesFlagName, flags.TailFlagName},
			Prepare: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) (context.Context, error) {
				workload := &cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Status: cartov1alpha1.WorkloadStatus{
						Conditions: []metav1.Condition{
							{
								Type:   cartov1alpha1.WorkloadConditionReady,
								Status: metav1.ConditionTrue,
							},
						},
					},
				}
				fakeWatcher := watchfakes.NewFakeWithWatch(false, config.Client, []watch.Event{
					{Type: watch.Modified, Object: workload},
				})
				ctx = watchhelper.WithWatcher(ctx, fakeWatcher)

				tailer := &logs.FakeTailer{}
				selector, _ := labels.Parse(fmt.Sprintf("%s=%s", cartov1alpha1.WorkloadLabelName, workloadName))
				tailer.On("Tail", mock.Anything, "default", selector, []string{}, time.Second, false).Return(nil).Once()
				ctx = logs.StashTailer(ctx, tailer)

				return ctx, nil
			},
			CleanUp: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) error {
				tailer := logs.RetrieveTailer(ctx).(*logs.FakeTailer)
				tailer.AssertExpectations(t)
				return nil
			},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						ServiceClaims: []cartov1alpha1.WorkloadServiceClaim{
							{
								Name: "database",
								Ref: &cartov1alpha1.WorkloadServiceClaimReference{
									APIVersion: "services.tanzu.vmware.com/v1alpha1",
									Kind:       "PostgreSQL",
									Name:       "my-prod-db",
								},
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7,  7   |spec:
  8,  8   |  image: ubuntu:bionic
      9 + |  serviceClaims:
     10 + |  - name: database
     11 + |    ref:
     12 + |      apiVersion: services.tanzu.vmware.com/v1alpha1
     13 + |      kind: PostgreSQL
     14 + |      name: my-prod-db

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload"
To get status: "tanzu apps workload get my-workload"

Waiting for workload "my-workload" to become ready...
...tail output...
Workload "my-workload" is ready
`,
		},
		{
			Name: "tail with timestamp while waiting for ready condition",
			Args: []string{workloadName, flags.ServiceRefFlagName, "database=services.tanzu.vmware.com/v1alpha1:PostgreSQL:my-prod-db", flags.YesFlagName, flags.TailFlagName, flags.TailTimestampFlagName},
			Prepare: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) (context.Context, error) {
				workload := &cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Status: cartov1alpha1.WorkloadStatus{
						Conditions: []metav1.Condition{
							{
								Type:   cartov1alpha1.WorkloadConditionReady,
								Status: metav1.ConditionTrue,
							},
						},
					},
				}
				fakeWatcher := watchfakes.NewFakeWithWatch(false, config.Client, []watch.Event{
					{Type: watch.Modified, Object: workload},
				})
				ctx = watchhelper.WithWatcher(ctx, fakeWatcher)

				tailer := &logs.FakeTailer{}
				selector, _ := labels.Parse(fmt.Sprintf("%s=%s", cartov1alpha1.WorkloadLabelName, workloadName))
				tailer.On("Tail", mock.Anything, "default", selector, []string{}, time.Second, true).Return(nil).Once()
				ctx = logs.StashTailer(ctx, tailer)

				return ctx, nil
			},
			CleanUp: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) error {
				tailer := logs.RetrieveTailer(ctx).(*logs.FakeTailer)
				tailer.AssertExpectations(t)
				return nil
			},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},

			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Image: "ubuntu:bionic",
						ServiceClaims: []cartov1alpha1.WorkloadServiceClaim{
							{
								Name: "database",
								Ref: &cartov1alpha1.WorkloadServiceClaimReference{
									APIVersion: "services.tanzu.vmware.com/v1alpha1",
									Kind:       "PostgreSQL",
									Name:       "my-prod-db",
								},
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7,  7   |spec:
  8,  8   |  image: ubuntu:bionic
      9 + |  serviceClaims:
     10 + |  - name: database
     11 + |    ref:
     12 + |      apiVersion: services.tanzu.vmware.com/v1alpha1
     13 + |      kind: PostgreSQL
     14 + |      name: my-prod-db

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload"
To get status: "tanzu apps workload get my-workload"

Waiting for workload "my-workload" to become ready...
...tail output...
Workload "my-workload" is ready
`,
		},
		{
			Name: "filepath",
			Args: []string{flags.FilePathFlagName, "testdata/workload.yaml", flags.YesFlagName},
			GivenObjects: []client.Object{
				sprintPetclinicWorkload.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.AddLabel("preserve-me", "should-exist")
					}).
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
						d.Env(corev1.EnvVar{
							Name:  "OVERRIDE_VAR",
							Value: "doesnt matter",
						})
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      "spring-petclinic",
						Labels: map[string]string{
							"preserve-me":                         "should-exist",
							"app.kubernetes.io/part-of":           "spring-petclinic",
							"apps.tanzu.vmware.com/workload-type": "web",
						},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "OVERRIDE_VAR",
								Value: "doesnt matter",
							},
							{
								Name:  "SPRING_PROFILES_ACTIVE",
								Value: "mysql",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  2,  2   |apiVersion: carto.run/v1alpha1
  3,  3   |kind: Workload
  4,  4   |metadata:
  5,  5   |  labels:
      6 + |    app.kubernetes.io/part-of: spring-petclinic
      7 + |    apps.tanzu.vmware.com/workload-type: web
  6,  8   |    preserve-me: should-exist
  7,  9   |  name: spring-petclinic
  8, 10   |  namespace: default
  9, 11   |spec:
 10, 12   |  env:
 11, 13   |  - name: OVERRIDE_VAR
 12, 14   |    value: doesnt matter
 13     - |  image: ubuntu:bionic
     15 + |  - name: SPRING_PROFILES_ACTIVE
     16 + |    value: mysql
     17 + |  resources:
     18 + |    limits:
     19 + |      cpu: 500m
     20 + |      memory: 1Gi
     21 + |    requests:
     22 + |      cpu: 100m
     23 + |      memory: 1Gi
     24 + |  source:
     25 + |    git:
     26 + |      ref:
     27 + |        branch: main
     28 + |      url: https://github.com/spring-projects/spring-petclinic.git

Updated workload "spring-petclinic"

To see logs:   "tanzu apps workload tail spring-petclinic"
To get status: "tanzu apps workload get spring-petclinic"

`,
		},
		{
			Name: "accept yaml file through stdin - using --yes flag",
			Args: []string{flags.FilePathFlagName, "-", flags.YesFlagName},
			Stdin: []byte(`
apiVersion: carto.run/v1alpha1
kind: Workload
metadata:
  name: spring-petclinic
  labels:
    app.kubernetes.io/part-of: spring-petclinic
    apps.tanzu.vmware.com/workload-type: web
spec:
  env:
  - name: SPRING_PROFILES_ACTIVE
    value: mysql
  resources:
    requests:
      memory: 1Gi
      cpu: 100m
    limits:
      memory: 1Gi
      cpu: 500m
  source:
    git:
      url: https://github.com/spring-projects/spring-petclinic.git
      ref:
        branch: main
`),
			GivenObjects: []client.Object{
				sprintPetclinicWorkload.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.AddLabel("preserve-me", "should-exist")
					}).
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
						d.Env(corev1.EnvVar{
							Name:  "OVERRIDE_VAR",
							Value: "doesnt matter",
						})
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      "spring-petclinic",
						Labels: map[string]string{
							"preserve-me":                         "should-exist",
							"app.kubernetes.io/part-of":           "spring-petclinic",
							"apps.tanzu.vmware.com/workload-type": "web",
						},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "OVERRIDE_VAR",
								Value: "doesnt matter",
							},
							{
								Name:  "SPRING_PROFILES_ACTIVE",
								Value: "mysql",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  2,  2   |apiVersion: carto.run/v1alpha1
  3,  3   |kind: Workload
  4,  4   |metadata:
  5,  5   |  labels:
      6 + |    app.kubernetes.io/part-of: spring-petclinic
      7 + |    apps.tanzu.vmware.com/workload-type: web
  6,  8   |    preserve-me: should-exist
  7,  9   |  name: spring-petclinic
  8, 10   |  namespace: default
  9, 11   |spec:
 10, 12   |  env:
 11, 13   |  - name: OVERRIDE_VAR
 12, 14   |    value: doesnt matter
 13     - |  image: ubuntu:bionic
     15 + |  - name: SPRING_PROFILES_ACTIVE
     16 + |    value: mysql
     17 + |  resources:
     18 + |    limits:
     19 + |      cpu: 500m
     20 + |      memory: 1Gi
     21 + |    requests:
     22 + |      cpu: 100m
     23 + |      memory: 1Gi
     24 + |  source:
     25 + |    git:
     26 + |      ref:
     27 + |        branch: main
     28 + |      url: https://github.com/spring-projects/spring-petclinic.git

Updated workload "spring-petclinic"

To see logs:   "tanzu apps workload tail spring-petclinic"
To get status: "tanzu apps workload get spring-petclinic"

`,
		},
		{
			Name: "accept yaml file through stdin - using --dry-run flag",
			Args: []string{flags.FilePathFlagName, "-", flags.DryRunFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
					}),
			},
			Stdin: []byte(`
---
apiVersion: carto.run/v1alpha1
kind: Workload
metadata:
  creationTimestamp: null
  name: my-workload
  namespace: default
  resourceVersion: "999"
spec:
  image: ubuntu:bionic
status:
  supplyChainRef: {}
`),
		},
		{
			Name: "fail to accept yaml file - missing --yes flag",
			Args: []string{flags.FilePathFlagName, "-"},
			Stdin: []byte(`
apiVersion: carto.run/v1alpha2
kind: Workload
metadata:
  name: spring-petclinic
  labels:
    app.kubernetes.io/part-of: spring-petclinic
    apps.tanzu.vmware.com/workload-type: web
spec:
  env:
  - name: SPRING_PROFILES_ACTIVE
    value: mysql
  resources:
    requests:
      memory: 1Gi
      cpu: 100m
    limits:
      memory: 1Gi
      cpu: 500m
  source:
    git:
      url: https://github.com/spring-projects/spring-petclinic.git
      ref:
        branch: main

`),
			ShouldError: true,
		},
		{
			Name: "filepath - custom namespace and name",
			Args: []string{workloadName, flags.NamespaceFlagName, "test-namespace", flags.FilePathFlagName, "testdata/workload.yaml", flags.YesFlagName},
			GivenObjects: []client.Object{
				diecartov1alpha1.WorkloadBlank.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Namespace("test-namespace")
						d.Name(workloadName)
						d.AddLabel("preserve-me", "should-exist")
					}).
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Image("ubuntu:bionic")
						d.Env(corev1.EnvVar{
							Name:  "OVERRIDE_VAR",
							Value: "doesnt matter",
						})
					},
					)},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-namespace",
						Name:      workloadName,
						Labels: map[string]string{
							"preserve-me":                         "should-exist",
							"app.kubernetes.io/part-of":           "spring-petclinic",
							"apps.tanzu.vmware.com/workload-type": "web",
						},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "OVERRIDE_VAR",
								Value: "doesnt matter",
							},
							{
								Name:  "SPRING_PROFILES_ACTIVE",
								Value: "mysql",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  2,  2   |apiVersion: carto.run/v1alpha1
  3,  3   |kind: Workload
  4,  4   |metadata:
  5,  5   |  labels:
      6 + |    app.kubernetes.io/part-of: spring-petclinic
      7 + |    apps.tanzu.vmware.com/workload-type: web
  6,  8   |    preserve-me: should-exist
  7,  9   |  name: my-workload
  8, 10   |  namespace: test-namespace
  9, 11   |spec:
 10, 12   |  env:
 11, 13   |  - name: OVERRIDE_VAR
 12, 14   |    value: doesnt matter
 13     - |  image: ubuntu:bionic
     15 + |  - name: SPRING_PROFILES_ACTIVE
     16 + |    value: mysql
     17 + |  resources:
     18 + |    limits:
     19 + |      cpu: 500m
     20 + |      memory: 1Gi
     21 + |    requests:
     22 + |      cpu: 100m
     23 + |      memory: 1Gi
     24 + |  source:
     25 + |    git:
     26 + |      ref:
     27 + |        branch: main
     28 + |      url: https://github.com/spring-projects/spring-petclinic.git

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload --namespace test-namespace"
To get status: "tanzu apps workload get my-workload --namespace test-namespace"

`,
		},
		{
			Name:        "filepath - missing",
			Args:        []string{workloadName, flags.FilePathFlagName, "testdata/missing.yaml", flags.YesFlagName},
			ShouldError: true,
		},
		{
			Name: "local path - missing fields",
			Args: []string{workloadName, flags.LocalPathFlagName, "testdata/local-source", flags.YesFlagName},
			GivenObjects: []client.Object{
				parent,
			},
			ShouldError: true,
			CleanUp: func(t *testing.T, ctx context.Context, config *cli.Config, tc *clitesting.CommandTestCase) error {
				if expected, actual := false, cmd.SilenceUsage; expected != actual {
					t.Errorf("expected cmd.SilenceUsage to be %t, actually %t", expected, actual)
				}

				return nil
			},
		},
		{
			Name: "update existing param-yaml",
			Args: []string{workloadName, flags.ParamYamlFlagName, `ports_json={"name": "smtp", "port": 2026}`, flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Source(
							&cartov1alpha1.Source{
								Git: &cartov1alpha1.GitSource{
									URL: "https://github.com/spring-projects/spring-petclinic.git",
									Ref: cartov1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						).Params(
							cartov1alpha1.Param{
								Name:  "ports_json",
								Value: apiextensionsv1.JSON{Raw: []byte(`{"name":"smtp","port":1026}`)},
							}, cartov1alpha1.Param{
								Name:  "ports_nesting_yaml",
								Value: apiextensionsv1.JSON{Raw: []byte(`[{"deployment":{"name":"smtp","port":1026}}]`)},
							},
						)
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
						},
						Params: []cartov1alpha1.Param{
							{
								Name:  "ports_json",
								Value: apiextensionsv1.JSON{Raw: []byte(`{"name":"smtp","port":2026}`)},
							}, {
								Name:  "ports_nesting_yaml",
								Value: apiextensionsv1.JSON{Raw: []byte(`[{"deployment":{"name":"smtp","port":1026}}]`)},
							},
						},
					},
				},
			},
		},
		{
			ShouldError: true,
			Name:        "fails create with multiple param-yaml using invalid json",
			Args: []string{flags.FilePathFlagName, "testdata/param-yaml.yaml",
				flags.ParamYamlFlagName, `ports_json={"name": "smtp", "port": 1026`,
				flags.ParamYamlFlagName, `ports_nesting_yaml=- deployment:\n    name: smtp\n    port: 1026`,
				flags.YesFlagName},
			ExpectOutput: "",
		},
		{
			Name: "update existing param-yaml from file",
			Args: []string{workloadName, flags.FilePathFlagName, "testdata/workload-param-yaml.yaml", flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {
						d.Source(
							&cartov1alpha1.Source{
								Git: &cartov1alpha1.GitSource{
									URL: "https://github.com/spring-projects/spring-petclinic.git",
									Ref: cartov1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						).Params(
							cartov1alpha1.Param{
								Name:  "ports",
								Value: apiextensionsv1.JSON{Raw: []byte(`{"name":"smtp","port":1026}`)},
							}, cartov1alpha1.Param{
								Name:  "ports_nesting_yaml",
								Value: apiextensionsv1.JSON{Raw: []byte(`[{"deployment":{"name":"smtp","port":1026}}]`)},
							},
						)
					}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels: map[string]string{
							apis.AppPartOfLabelName:               "spring-petclinic",
							"apps.tanzu.vmware.com/workload-type": "web",
						},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Source: &cartov1alpha1.Source{
							Git: &cartov1alpha1.GitSource{
								URL: "https://github.com/spring-projects/spring-petclinic.git",
								Ref: cartov1alpha1.GitRef{
									Branch: "main",
								},
							},
						},
						Params: []cartov1alpha1.Param{
							{
								Name:  "ports",
								Value: apiextensionsv1.JSON{Raw: []byte(`{"ports":[{"name":"http","port":8080,"protocol":"TCP","targetPort":8080},{"name":"https","port":8443,"protocol":"TCP","targetPort":8443}]}`)},
							}, {
								Name:  "ports_nesting_yaml",
								Value: apiextensionsv1.JSON{Raw: []byte(`[{"deployment":{"name":"smtp","port":1026}}]`)},
							}, {
								Name:  "services",
								Value: apiextensionsv1.JSON{Raw: []byte(`[{"image":"mysql:5.7","name":"mysql"},{"image":"postgres:9.6","name":"postgres"}]`)},
							},
						},
					},
				},
			},
		},
		{
			Name: "update workload to add maven param",
			Args: []string{workloadName, flags.ParamYamlFlagName, `maven={"artifactId": "spring-petclinic", "version": "2.6.0", "groupId": "org.springframework.samples"}`, flags.YesFlagName},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diecartov1alpha1.WorkloadSpecDie) {}),
			},
			ExpectUpdates: []client.Object{
				&cartov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNamespace,
						Name:      workloadName,
						Labels:    map[string]string{},
					},
					Spec: cartov1alpha1.WorkloadSpec{
						Params: []cartov1alpha1.Param{
							{
								Name:  "maven",
								Value: apiextensionsv1.JSON{Raw: []byte(`{"artifactId":"spring-petclinic","groupId":"org.springframework.samples","version":"2.6.0"}`)},
							},
						},
					},
				},
			},
			ExpectOutput: `
WARNING: the update command has been deprecated and will be removed in a future update. Please use "tanzu apps workload apply" instead.

Update workload:
...
  3,  3   |kind: Workload
  4,  4   |metadata:
  5,  5   |  name: my-workload
  6,  6   |  namespace: default
  7     - |spec: {}
      7 + |spec:
      8 + |  params:
      9 + |  - name: maven
     10 + |    value:
     11 + |      artifactId: spring-petclinic
     12 + |      groupId: org.springframework.samples
     13 + |      version: 2.6.0

Updated workload "my-workload"

To see logs:   "tanzu apps workload tail my-workload"
To get status: "tanzu apps workload get my-workload"

`,
		},
	}

	table.Run(t, scheme, func(ctx context.Context, c *cli.Config) *cobra.Command {
		// capture the cobra command so we can make assertions on cleanup, this will fail if tests are run parallel.
		cmd = commands.NewWorkloadUpdateCommand(ctx, c)
		return cmd
	})
}
