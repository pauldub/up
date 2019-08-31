package deployment

import (
	"context"

	"github.com/apex/up"
	"github.com/apex/up/platform/event"
	"github.com/apex/up/platform/kubernetes/build"
	"github.com/apex/up/platform/kubernetes/stack"
	"github.com/ericchiang/k8s"
	appsv1 "github.com/ericchiang/k8s/apis/apps/v1"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
	"github.com/ericchiang/k8s/util/intstr"
	"github.com/pkg/errors"
)

type Deployment struct {
	stack  stack.Stack
	build  *build.Build
	config *up.Config
	events event.Events
	info   up.Deploy
}

func New(
	stack stack.Stack, build *build.Build,
	config *up.Config, events event.Events, deploy up.Deploy,
) *Deployment {
	return &Deployment{
		stack:  stack,
		build:  build,
		config: config,
		events: events,
		info:   deploy,
	}
}

func (d *Deployment) Deploy(ctx context.Context) error {
	var operation func(
		ctx context.Context, req k8s.Resource, options ...k8s.Option,
	) error = d.stack.K8s().Update

	var previousDeploy appsv1.Deployment
	err := d.stack.K8s().Get(ctx, d.stack.Namespace(), d.deploymentName(), &previousDeploy)
	if err != nil {
		operation = d.stack.K8s().Create
	}

	deployment := d.deployment()

	err = operation(ctx, deployment)
	if err != nil {
		return errors.Wrap(err, "deployment apply")
	}

	label := &k8s.LabelSelector{}
	label.Eq("up-build-id", d.build.ID)
	label.Eq("up-process", "deploy")

	watcher, err := d.stack.K8s().Watch(
		ctx, d.stack.Namespace(), deployment, label.Selector(),
	)
	if err != nil {
		return errors.Wrap(err, "watch deploy")
	}
	defer watcher.Close()

	for {
		deploy := new(appsv1.Deployment)
		_, err := watcher.Next(deploy)
		if err != nil {
			return errors.Wrap(err, "watch next")
		}

		if *deploy.Status.AvailableReplicas == *deploy.Status.Replicas {
			watcher.Close()
			break
		}
	}

	operation = d.stack.K8s().Update

	var (
		previousService corev1.Service
		previousIP      = ""
		resourceVersion = ""
	)
	err = d.stack.K8s().Get(ctx, d.stack.Namespace(), d.serviceName(), &previousService)
	if err != nil {
		operation = d.stack.K8s().Create
	} else {
		previousIP = *previousService.Spec.ClusterIP
		resourceVersion = *previousService.Metadata.ResourceVersion
	}

	service := d.service(previousIP, resourceVersion)

	err = operation(ctx, service)
	if err != nil {
		return errors.Wrap(err, "deployment apply")
	}

	return nil
}

func (d *Deployment) deployment() *appsv1.Deployment {
	kubernetes := d.config.Kubernetes

	return &appsv1.Deployment{
		Metadata: &metav1.ObjectMeta{
			Name:      k8s.String(d.deploymentName()),
			Namespace: k8s.String(d.stack.Namespace()),
			Labels:    d.deploymentLabels(),
		},
		Spec: &appsv1.DeploymentSpec{
			Replicas: k8s.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: d.podLabels(),
			},
			Template: &corev1.PodTemplateSpec{
				Metadata: &metav1.ObjectMeta{
					Labels: d.podLabels(),
				},
				Spec: &corev1.PodSpec{
					Containers: []*corev1.Container{
						&corev1.Container{
							Name: k8s.String(d.podName()),
							Image: k8s.String(
								d.build.Image(kubernetes.Registry.URL, kubernetes.Registry.Image),
							),
							Env: []*corev1.EnvVar{
								&corev1.EnvVar{
									Name:  k8s.String("AWS_LAMBDA_FUNCTION_NAME"),
									Value: k8s.String(d.config.Name),
								},
								&corev1.EnvVar{
									Name:  k8s.String("AWS_LAMBDA_FUNCTION_VERSION"),
									Value: k8s.String(d.info.Commit),
								},
								&corev1.EnvVar{
									Name:  k8s.String("UP_STAGE"),
									Value: k8s.String(d.info.Stage),
								},
							},
						},
					},
					ImagePullSecrets: []*corev1.LocalObjectReference{
						&corev1.LocalObjectReference{
							Name: k8s.String(stack.DockerRegistrySecret),
						},
					},
				},
			},
		},
	}
}

func (d *Deployment) deploymentName() string {
	return d.config.Name
}

func (d *Deployment) deploymentLabels() map[string]string {
	return map[string]string{
		"up-project":  d.config.Name,
		"up-stage":    d.info.Stage,
		"up-process":  "deploy",
		"up-build-id": d.build.ID,
	}
}

func (d *Deployment) serviceName() string {
	return d.config.Name
}

func (d *Deployment) podName() string {
	return d.config.Name
}

func (d *Deployment) podLabels() map[string]string {
	return map[string]string{
		"up-project": d.config.Name,
		"up-stage":   d.info.Stage,
		"up-process": "deploy",
	}
}

func (d *Deployment) service(previousIP string, resourceVersion string) *corev1.Service {
	var portType int64 = 0
	return &corev1.Service{
		Metadata: &metav1.ObjectMeta{
			Name:            k8s.String(d.serviceName()),
			Namespace:       k8s.String(d.stack.Namespace()),
			ResourceVersion: k8s.String(resourceVersion),
			Labels: map[string]string{
				"up-project": d.config.Name,
				"up-stage":   d.info.Stage,
				"up-process": "deploy",
			},
		},
		Spec: &corev1.ServiceSpec{
			Ports: []*corev1.ServicePort{
				&corev1.ServicePort{
					Name: k8s.String("up-proxy"),
					// Procotol:   k8s.String("http"),
					Port: k8s.Int32(80),
					TargetPort: &intstr.IntOrString{
						Type:   &portType,
						IntVal: k8s.Int32(8080),
					},
				},
			},
			Selector:  d.podLabels(),
			Type:      k8s.String("ClusterIP"),
			ClusterIP: k8s.String(previousIP),
		},
	}
}
