package stack

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/apex/up"
	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
	minio "github.com/minio/minio-go"
	"github.com/pkg/errors"
)

const (
	DockerRegistrySecret     = "docker-registry"
	storageCredentialsSecret = "storage-credentials"
)

type Stack interface {
	Namespace() string
	K8s() *k8s.Client
	Storage() *minio.Client
	Config() *up.Config
}

type KubernetesStack struct {
	Name    string
	config  *up.Config
	k8s     *k8s.Client
	storage *minio.Client
}

func New(
	name string, config *up.Config,
	k8sClient *k8s.Client, storage *minio.Client,
) *KubernetesStack {
	return &KubernetesStack{
		Name:    name,
		config:  config,
		k8s:     k8sClient,
		storage: storage,
	}
}

func (s *KubernetesStack) Namespace() string {
	return s.Name
}

func (s *KubernetesStack) K8s() *k8s.Client {
	return s.k8s
}

func (s *KubernetesStack) Storage() *minio.Client {
	return s.storage
}

func (s *KubernetesStack) Config() *up.Config {
	return s.config
}

func (s *KubernetesStack) Create(
	ctx context.Context,
) error {
	err := s.k8s.Create(
		ctx, &corev1.Namespace{
			Metadata: &metav1.ObjectMeta{
				Name: k8s.String(s.Name),
			},
		},
	)
	if apiErr, ok := err.(*k8s.APIError); ok {
		if *apiErr.Status.Code == 409 {
			goto namespaceExists
		}

		return errors.Wrap(err, "create namespace")
	}
namespaceExists:

	if s.config.Docker.Registry.Password != "" {
		err = s.createDockerRegistrySecret(ctx)
		if err != nil {
			return errors.Wrap(err, "create registry secret")
		}
	}

	/* err = p.createStorageCredentialsSecret(ctx)
	if err != nil {
		return errors.Wrap(err, "create storage secret")
	}*/

	return nil
}

func (s *KubernetesStack) createDockerRegistrySecret(
	ctx context.Context,
) error {
	docker := s.config.Docker

	registryAuth := base64.StdEncoding.EncodeToString(
		[]byte(
			fmt.Sprintf("%s:%s",
				docker.Registry.Username,
				docker.Registry.Password,
			),
		),
	)

	dockercfg := fmt.Sprintf(
		`{"%s":{"username":"%s","password":"%s","email":"%s","auth":"%s"}}`,
		docker.Registry.URL,
		docker.Registry.Username,
		docker.Registry.Password,
		docker.Registry.Email,
		registryAuth,
	)

	dockerConfig := fmt.Sprintf(
		`{"auths":{"%s":{"auth":"%s"}}}`,
		docker.Registry.URL, registryAuth,
	)

	var secret corev1.Secret

	err := s.k8s.Get(ctx, s.Name, DockerRegistrySecret, &secret)
	if err != nil {
		return errors.WithStack(
			s.k8s.Create(
				ctx, &corev1.Secret{
					Metadata: &metav1.ObjectMeta{
						Name:      k8s.String(DockerRegistrySecret),
						Namespace: k8s.String(s.Name),
					},
					Type: k8s.String("kubernetes.io/dockercfg"),
					StringData: map[string]string{
						".dockercfg":  dockercfg,
						"config.json": dockerConfig,
					},
				},
			),
		)
	}

	secret.StringData = map[string]string{
		".dockercfg":  dockercfg,
		"config.json": dockerConfig,
	}

	return errors.WithStack(s.k8s.Update(ctx, &secret))
}

/*func (p *Platform) createStorageCredentialsSecret(ctx context.Context) error {
	credentials := fmt.Sprintf(
		`[default]
aws_access_key_id = %s
aws_secret_acess_key = %s`,
		p.config.Kubernetes.Storage.AccessKey,
		p.config.Kubernetes.Storage.SecretKey,
	)

	config := fmt.Sprintf(
		`[default]
region = us-east-1
s3 =
    endpoint = https://%s
    signature_version = s3v4`,
		p.config.Kubernetes.Storage.Endpoint,
	)
	fmt.Println(config)

	var secret corev1.Secret

	err := p.k8s.Get(ctx, p.projectNamespace(), storageCredentialsSecret, &secret)
	if err != nil {
		return errors.WithStack(
			p.k8s.Create(
				ctx, &corev1.Secret{
					Metadata: &metav1.ObjectMeta{
						Name:      k8s.String(storageCredentialsSecret),
						Namespace: k8s.String(p.projectNamespace()),
					},
					Type: k8s.String("Opaque"),
					StringData: map[string]string{
						"config":      config,
						"credentials": credentials,
					},
				},
			),
		)
	}

	secret.StringData = map[string]string{
		"config":      config,
		"credentials": credentials,
	}

	return errors.WithStack(p.k8s.Update(ctx, &secret))
}*/
