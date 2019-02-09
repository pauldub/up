package kubeconfig

import (
	"io/ioutil"

	"github.com/ericchiang/k8s"
	"github.com/ghodss/yaml"
	homedir "github.com/mitchellh/go-homedir"
)

// LoadFile parses a kubeconfig from a file and returns a Kubernetes
// client. It does not support extensions or client auth providers.
func LoadFile(kubeConfigFile string) (*k8s.Config, error) {
	var data []byte
	var err error

	kubeConfigFile, err = homedir.Expand(kubeConfigFile)
	if err != nil {
		return nil, err
	}

	if data, err = ioutil.ReadFile(kubeConfigFile); err != nil {
		return nil, err
	}

	// Unmarshal YAML into a Kubernetes config object.
	var config k8s.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
