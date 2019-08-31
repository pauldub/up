package config

import (
	"os"

	"github.com/apex/up/internal/validate"
	"github.com/apex/up/platform/kubernetes/kubeconfig"
	"github.com/pkg/errors"
)

type Kubernetes struct {
	KubeConfig  string `json:"kube_config"`
	KubeContext string `json:"kube_context"`
	Storage     struct {
		Endpoint  string `json:"endpoint"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
		Secure    bool   `json:"secure"`
		Bucket    string `json:"bucket"`
		Location  string `json:"location"`
	} `json:"storage"`
	Registry struct {
		URL      string `json:"url"`
		Image    string `json:"image"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"registry"`
}

func (d *Kubernetes) Validate() error {
	if err := validate.RequiredString(d.KubeConfig); err != nil {
		return errors.Wrap(err, ".kube_config")
	}

	config, err := kubeconfig.LoadFile(d.KubeConfig)
	if err != nil {
		return errors.Wrap(err, ".kube_config")
	}

	if err := validate.RequiredString(d.KubeContext); err != nil {
		return errors.Wrap(err, ".kube_context")
	}

	contextFound := false
	for _, ctx := range config.Contexts {
		if ctx.Name == d.KubeContext {
			contextFound = true
			break
		}
	}

	if !contextFound {
		return errors.New(".kube_context not found")
	}

	if err := validate.RequiredString(d.Storage.Endpoint); err != nil {
		return errors.Wrap(err, ".storage: .enpdoint")
	}

	if err := validate.RequiredString(d.Storage.AccessKey); err != nil {
		return errors.Wrap(err, ".storage: .access_key")
	}

	if err := validate.RequiredString(d.Storage.SecretKey); err != nil {
		return errors.Wrap(err, ".storage: .secret_key")
	}

	if err := validate.RequiredString(d.Storage.Bucket); err != nil {
		return errors.Wrap(err, ".storage: .bucket")
	}

	return nil
}

func (d *Kubernetes) Default() error {
	envKubeConfig := os.Getenv("KUBE_CONFIG")
	if envKubeConfig != "" {
		d.KubeConfig = envKubeConfig
	}

	if d.KubeConfig == "" {
		d.KubeConfig = "~/.kube/config"
	}

	envRegistryURL := os.Getenv("DOCKER_REGISTRY_URL")
	if envRegistryURL != "" {
		d.Registry.URL = envRegistryURL
	}

	envRegistryImage := os.Getenv("DOCKER_REGISTRY_IMAGE")
	if envRegistryImage != "" {
		d.Registry.Image = envRegistryImage
	}

	envRegistryUsername := os.Getenv("DOCKER_REGISTRY_USERNAME")
	if envRegistryUsername != "" {
		d.Registry.Username = envRegistryUsername
	}

	envRegistryEmail := os.Getenv("DOCKER_REGISTRY_EMAIL")
	if envRegistryEmail != "" {
		d.Registry.Email = envRegistryEmail
	}

	envRegistryPassword := os.Getenv("DOCKER_REGISTRY_PASSWORD")
	if envRegistryPassword != "" {
		d.Registry.Password = envRegistryPassword
	}

	return nil
}

// Override config.
func (k *Kubernetes) Override(c *Config) {
	if k.KubeConfig != "" {
		c.Kubernetes.KubeConfig = k.KubeConfig
	}

	if k.KubeContext != "" {
		c.Kubernetes.KubeContext = k.KubeContext
	}

	if k.Storage.Endpoint != "" {
		c.Kubernetes.Storage.Endpoint = k.Storage.Endpoint
	}

	if k.Storage.AccessKey != "" {
		c.Kubernetes.Storage.AccessKey = k.Storage.AccessKey
	}

	if k.Storage.SecretKey != "" {
		c.Kubernetes.Storage.SecretKey = k.Storage.SecretKey
	}

	if k.Storage.Secure != false {
		c.Kubernetes.Storage.Secure = k.Storage.Secure
	}

	if k.Storage.Bucket != "" {
		c.Kubernetes.Storage.Secure = k.Storage.Secure
	}

	if k.Storage.Location != "" {
		c.Kubernetes.Storage.Location = k.Storage.Location
	}

	if k.Registry.URL != "" {
		c.Kubernetes.Registry.URL = k.Registry.URL
	}

	if k.Registry.Image != "" {
		c.Kubernetes.Registry.Image = k.Registry.Image
	}

	if k.Registry.Username != "" {
		c.Kubernetes.Registry.Username = k.Registry.Username
	}

	if k.Registry.Email != "" {
		c.Kubernetes.Registry.Email = k.Registry.Email
	}

	if k.Registry.Password != "" {
		c.Kubernetes.Registry.Password = k.Registry.Password
	}
}
