package config

import (
	"os"

	"github.com/apex/up/internal/validate"
	"github.com/pkg/errors"
)

type Docker struct {
	Dockerfile string `json:"dockerfile"`
	Registry   struct {
		URL      string `json:"url"`
		Image    string `json:"image"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"registry"`
}

func (d *Docker) Validate() error {
	if err := validate.RequiredString(d.Dockerfile); err != nil {
		return errors.Wrap(err, ".dockerfile")
	}

	_, err := os.Open(d.Dockerfile)
	if err != nil {
		return errors.Wrap(err, ".dockerfile")
	}
	return nil
}

func (d *Docker) Default(projectName string) error {
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
