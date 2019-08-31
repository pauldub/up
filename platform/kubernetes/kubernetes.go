package kubernetes

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/apex/up"
	"github.com/apex/up/platform/event"
	"github.com/apex/up/platform/kubernetes/build"
	"github.com/apex/up/platform/kubernetes/deployment"
	"github.com/apex/up/platform/kubernetes/kubeconfig"
	"github.com/apex/up/platform/kubernetes/stack"
	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	minio "github.com/minio/minio-go"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
	kcorev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Platform struct {
	config *up.Config
	events event.Events

	stage   string
	build   *build.Build
	tarball *bytes.Buffer

	stack *stack.KubernetesStack
}

// New platform.
func New(c *up.Config, events event.Events) *Platform {
	return &Platform{
		config: c,
		events: events,
	}
}

func (p *Platform) Init(stage string) error {
	p.stage = stage

	config, err := kubeconfig.LoadFile(p.config.Kubernetes.KubeConfig)
	if err != nil {
		return errors.Wrap(err, "load kubeconfig")
	}

	k8sClient, err := k8s.NewClient(config)
	if err != nil {
		return errors.Wrap(err, "initialize k8s")
	}

	kubeConfigFile, err := homedir.Expand(p.config.Kubernetes.KubeConfig)
	if err != nil {
		return err
	}

	// use the current context in kubeconfig
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "initialize kubernetes clientset")
	}

	minioClient, err := minio.New(
		strings.TrimPrefix(
			strings.TrimPrefix(p.config.Kubernetes.Storage.Endpoint, "http://"),
			"https://"),
		p.config.Kubernetes.Storage.AccessKey,
		p.config.Kubernetes.Storage.SecretKey,
		p.config.Kubernetes.Storage.Secure,
	)
	if err != nil {
		return errors.Wrap(err, "initialize minio")
	}

	p.stack = stack.New(
		p.projectNamespace(), p.config, p.events, k8sClient, clientset, minioClient,
	)

	return nil
}

func (p *Platform) Build() error {
	start := time.Now()

	ctx := context.Background()

	if err := p.stack.Create(ctx); err != nil {
		return errors.Wrap(err, "create stack")
	}

	p.build = build.New(p.stage, p.stack)
	if err := p.build.Run(ctx); err != nil {
		return errors.Wrap(err, "build run")
	}

	p.events.Emit("platform.build.zip", event.Fields{
		"files":             p.build.Stats.FilesAdded,
		"size_uncompressed": p.build.Stats.SizeUncompressed,
		"size_compressed":   p.build.TarballSize,
		"duration":          time.Since(start),
	})

	return nil
}

func (p *Platform) projectNamespace() string {
	return fmt.Sprintf("up-%s-%s", p.config.Name, p.stage)
}

/* func (p *Platform) Zip() io.Reader {
	return p.targz
} */

func (p *Platform) Deploy(deploy up.Deploy) error {
	start := time.Now()

	ctx := context.Background()
	err := deployment.New(p.stack, p.build, p.config, p.events, deploy).Deploy(ctx)
	if err != nil {
		return errors.Wrap(err, "deployment deploy")
	}

	fields := event.Fields{
		"commit": deploy.Commit,
		"stage":  deploy.Stage,
	}

	defer func() {
		fields["duration"] = time.Since(start)
		fields["commit"] = deploy.Commit
		fields["version"] = p.build.ID
		p.events.Emit("platform.deploy.complete", fields)
	}()

	url, err := p.URL("", deploy.Stage)
	if err != nil {
		return errors.Wrap(err, "fetching url")
	}

	p.events.Emit("platform.deploy.url", event.Fields{
		"url": url,
	})

	return nil
}

func (p *Platform) Logs(l up.LogsConfig) up.Logs {
	litter.Dump(l)

	var (
		pods corev1.PodList
	)

	label := &k8s.LabelSelector{}
	label.Eq("up-project", p.config.Name)
	label.Eq("up-process", "deploy")

	err := p.stack.K8s().List(context.Background(), p.stack.Namespace(), &pods, label.Selector())
	if err != nil {
		return nil
	}

	readers := make([]io.Reader, 0)

	for _, pod := range pods.Items {
		req := p.stack.Client().CoreV1().Pods(p.stack.Namespace()).GetLogs(*pod.Metadata.Name, &kcorev1.PodLogOptions{})
		logs, err := req.Stream()

		if err != nil {
			return nil
		}
		defer logs.Close()

		readers = append(readers, logs)
	}

	scanner := bufio.NewScanner(io.MultiReader(readers...))

	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	panic("not implemented")
}

func (p *Platform) Domains() up.Domains {
	panic("not implemented")
}

func (p *Platform) URL(region, stage string) (string, error) {
	var (
		service corev1.Service
	)

	err := p.stack.K8s().Get(context.Background(), p.stack.Namespace(), p.config.Name, &service)
	if err != nil {
		return "", errors.Wrap(err, "URL")
	}

	return *service.Spec.ClusterIP, nil
}

func (p *Platform) Exists(region string) (bool, error) {
	panic("not implemented")
}

func (p *Platform) CreateStack(region string, version string) error {
	panic("not implemented")
}

func (p *Platform) DeleteStack(region string, wait bool) error {
	panic("not implemented")
}

func (p *Platform) ShowStack(region string) error {
	panic("not implemented")
}

func (p *Platform) PlanStack(region string) error {
	panic("not implemented")
}

func (p *Platform) ApplyStack(region string) error {
	panic("not implemented")
}

func (p *Platform) ShowMetrics(region string, stage string, start time.Time) error {
	panic("not implemented")
}
