package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/apex/log"
	"github.com/apex/up"
	"github.com/apex/up/internal/proxy/bin"
	"github.com/apex/up/internal/targz"
	"github.com/apex/up/platform/event"
	"github.com/apex/up/platform/kubernetes/build"
	"github.com/apex/up/platform/kubernetes/deployment"
	"github.com/apex/up/platform/kubernetes/kubeconfig"
	"github.com/apex/up/platform/kubernetes/stack"
	"github.com/ericchiang/k8s"
	minio "github.com/minio/minio-go"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
)

type Platform struct {
	config *up.Config
	events event.Events

	stage   string
	build   *build.Build
	tarball *bytes.Buffer

	stack   *stack.KubernetesStack
	k8s     *k8s.Client
	storage *minio.Client
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

	minioClient, err := minio.New(
		p.config.Kubernetes.Storage.Endpoint,
		p.config.Kubernetes.Storage.AccessKey,
		p.config.Kubernetes.Storage.SecretKey,
		p.config.Kubernetes.Storage.Secure,
	)
	if err != nil {
		return errors.Wrap(err, "initialize minio")
	}

	p.k8s = k8sClient
	p.storage = minioClient
	p.stack = stack.New(
		p.projectNamespace(), p.config, p.events, p.k8s, p.storage,
	)

	return nil
}

func (p *Platform) Build() error {
	start := time.Now()
	p.tarball = new(bytes.Buffer)

	if err := p.injectProxy(); err != nil {
		return errors.Wrap(err, "injecting proxy")
	}
	defer p.removeProxy()

	p.events.Emit("log", event.Fields{
		"message": "build start",
	})

	r, stats, err := targz.Build(".")
	if err != nil {
		return errors.Wrap(err, "tag.gz")
	}

	if _, err := io.Copy(p.tarball, r); err != nil {
		return errors.Wrap(err, "copying")
	}

	if err := r.Close(); err != nil {
		return errors.Wrap(err, "closing")
	}

	tarballSize := p.tarball.Len()

	p.events.Emit("log", event.Fields{
		"message":  "tarball complete",
		"duration": time.Since(start),
	})

	ctx := context.Background()

	if err := p.stack.Create(ctx); err != nil {
		return errors.Wrap(err, "create stack")
	}

	p.events.Emit("log", event.Fields{
		"message": "stack complete",
	})

	p.build = build.New(p.stage, ioutil.NopCloser(p.tarball), p.stack)
	if err := p.build.Run(ctx); err != nil {
		return errors.Wrap(err, "build run")
	}

	p.events.Emit("platform.build.zip", event.Fields{
		"files":             stats.FilesAdded,
		"size_uncompressed": stats.SizeUncompressed,
		"size_compressed":   tarballSize,
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
	litter.Dump(deploy)
	litter.Dump(p.build)

	ctx := context.Background()
	err := deployment.New(p.stack, p.build, p.events, deploy).Deploy(ctx)
	return errors.Wrap(err, "deployment deploy")
}

func (p *Platform) Logs(up.LogsConfig) up.Logs {
	panic("not implemented")
}

func (p *Platform) Domains() up.Domains {
	panic("not implemented")
}

func (p *Platform) URL(region string, stage string) (string, error) {
	panic("not implemented")
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

const runtimeDockerfile = `
FROM heroku/heroku:18

ADD . /app
WORKDIR /app

ENTRYPOINT ['./up-proxy']
`

// injectProxy injects the Go proxy.
func (p *Platform) injectProxy() error {
	log.Debugf("injecting proxy")

	if err := ioutil.WriteFile("up-proxy", bin.MustAsset("up-proxy"), 0777); err != nil {
		return errors.Wrap(err, "writing up-proxy")
	}

	if err := ioutil.WriteFile("Dockerfile.up", []byte(runtimeDockerfile), 0655); err != nil {
		return errors.Wrap(err, "writing up-proxy")
	}

	return nil
}

// removeProxy removes the Go proxy.
func (p *Platform) removeProxy() error {
	log.Debugf("removing proxy")
	os.Remove("up-proxy")
	os.Remove("Dockerfile.up")
	return nil
}

