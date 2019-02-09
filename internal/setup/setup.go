// Package setup provides up.json initialization.
package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/tj/go/term"
	"github.com/tj/survey"

	"github.com/apex/up"
	"github.com/apex/up/internal/util"
	"github.com/apex/up/internal/validate"
	"github.com/apex/up/platform/aws/regions"
	"github.com/apex/up/platform/kubernetes/kubeconfig"
)

// ErrNoCredentials is the error returned when no AWS credential profiles are available.
var ErrNoCredentials = errors.New("no credentials")

// config saved to up.json
type config struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Kubernetes struct {
		KubeContext string `json:"kube_context"`
	} `json:"kubernetes,omitempty"`
	Profile string   `json:"profile,omitempty"`
	Regions []string `json:"regions,omitempty"`
}

// questions for the user.
var questions = []*survey.Question{
	{
		Name: "name",
		Prompt: &survey.Input{
			Message: "Project name:",
			Default: defaultName()}, Validate: validateName,
	},
	{
		Name: "platform",
		Prompt: &survey.Select{
			Message:  "Platform:",
			Options:  platforms(),
			Default:  os.Getenv("PLATFORM"),
			PageSize: 10,
		},
		Validate: survey.Required,
	},
}

var awsQuestions = []*survey.Question{
	{
		Name: "profile",
		Prompt: &survey.Select{
			Message:  "AWS profile:",
			Options:  awsProfiles(),
			Default:  os.Getenv("AWS_PROFILE"),
			PageSize: 10,
		},
		Validate: survey.Required,
	},
	{
		Name: "region",
		Prompt: &survey.Select{
			Message:  "AWS region:",
			Options:  regions.Names,
			Default:  defaultRegion(),
			PageSize: 15,
		},
		Validate: survey.Required,
	},
}

var kubernetesQuestions = []*survey.Question{
	{
		Name: "kube_context",
		Prompt: &survey.Select{
			Message:  "Kubectl context: ",
			Options:  kubeContexts(),
			Default:  defaultKubeContext(),
			PageSize: 15,
		},
		Validate: survey.Required,
	},
}

// Create an up.json file for the user.
func Create() error {
	var in struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}

	var awsIn struct {
		Profile string `json:"profile"`
		Region  string `json:"region"`
	}

	var kubernetesIn struct {
		KubeContext string `json:"kube_context" survey:"kube_context"`
	}

	if len(awsProfiles()) == 0 {
		return ErrNoCredentials
	}

	println()

	// confirm
	var ok bool
	err := survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("No up.json found, create a new project?"),
		Default: true,
	}, &ok, nil)

	if err != nil {
		return err
	}

	if !ok {
		return errors.New("aborted")
	}

	// prompt
	term.MoveUp(1)
	term.ClearLine()
	if err := survey.Ask(questions, &in); err != nil {
		return err
	}

	c := config{
		Name:     in.Name,
		Platform: in.Platform,
	}

	switch in.Platform {
	case up.PlatformLambda:
		if err := survey.Ask(awsQuestions, &awsIn); err != nil {
			return err
		}

		c.Profile = awsIn.Profile
		c.Regions = []string{
			regions.GetIdByName(awsIn.Region),
		}
	case up.PlatformKubernetes:
		if err := survey.Ask(kubernetesQuestions, &kubernetesIn); err != nil {
			return err
		}

		c.Kubernetes.KubeContext = kubernetesIn.KubeContext
	}

	b, _ := json.MarshalIndent(c, "", "  ")
	return ioutil.WriteFile("up.json", b, 0644)
}

// defaultName returns the default app name.
// The name is only inferred if it is valid.
func defaultName() string {
	dir, _ := os.Getwd()
	name := filepath.Base(dir)
	if validate.Name(name) != nil {
		return ""
	}
	return name
}

// defaultRegion returns the default aws region.
func defaultRegion() string {
	if s := os.Getenv("AWS_DEFAULT_REGION"); s != "" {
		return s
	}

	if s := os.Getenv("AWS_REGION"); s != "" {
		return s
	}

	return ""
}

// validateName validates the name prompt.
func validateName(v interface{}) error {
	if err := validate.Name(v.(string)); err != nil {
		return err
	}

	return survey.Required(v)
}

// awsProfiles returns the AWS profiles found.
func awsProfiles() []string {
	path, err := homedir.Expand("~/.aws/credentials")
	if err != nil {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	s, err := util.ParseSections(f)
	if err != nil {
		return nil
	}

	sort.Strings(s)
	return s
}

func platforms() []string {
	return []string{
		up.PlatformLambda, up.PlatformKubernetes,
	}
}

func kubeContexts() []string {
	config, err := kubeconfig.LoadFile("~/.kube/config")
	if err != nil {
		return []string{}
	}

	contexts := make([]string, 0)
	for _, ctx := range config.Contexts {
		contexts = append(contexts, ctx.Name)
	}

	return contexts
}

func defaultKubeContext() string {
	envContext := os.Getenv("KUBE_CONTEXT")
	if envContext != "" {
		return envContext
	}

	contexts := kubeContexts()
	return contexts[0]
}
