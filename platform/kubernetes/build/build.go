package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/apex/log"
	"github.com/apex/up"
	"github.com/apex/up/internal/proxy/bin"
	"github.com/apex/up/internal/targz"
	"github.com/apex/up/platform/event"
	"github.com/apex/up/platform/kubernetes/stack"
	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
	minio "github.com/minio/minio-go"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	archive "github.com/tj/go-archive"
)

type Build struct {
	ID          string
	Stage       string
	Tarball     io.ReadCloser
	TarballSize int
	Stats       *archive.Stats

	stack   stack.Stack
	k8s     *k8s.Client
	storage *minio.Client
	config  *up.Config
	events  event.Events
}

func New(
	stage string, stack stack.Stack,
) *Build {
	return &Build{
		ID:      xid.New().String(),
		Stage:   stage,
		stack:   stack,
		k8s:     stack.K8s(),
		storage: stack.Storage(),
		config:  stack.Config(),
		events:  stack.Events(),
	}
}

func (b *Build) tarball() (*bytes.Buffer, error) {
	tarball := new(bytes.Buffer)

	if err := b.injectProxy(); err != nil {
		return tarball, errors.Wrap(err, "injecting proxy")
	}
	defer b.removeProxy()

	r, stats, err := targz.Build(".")
	if err != nil {
		return tarball, errors.Wrap(err, "tag.gz")
	}

	if _, err := io.Copy(tarball, r); err != nil {
		return tarball, errors.Wrap(err, "copying")
	}

	if err := r.Close(); err != nil {
		return tarball, errors.Wrap(err, "closing")
	}

	b.Stats = stats
	b.Tarball = ioutil.NopCloser(tarball)
	b.TarballSize = tarball.Len()

	return tarball, nil
}

func (b *Build) upload() (string, error) {
	var (
		kubernetes = b.config.Kubernetes
	)

	exists, err := b.storage.BucketExists(kubernetes.Storage.Bucket)
	if err != nil {
		return "", errors.Wrap(err, "bucket exists")
	}

	if !exists {
		err := b.storage.MakeBucket(
			kubernetes.Storage.Bucket,
			kubernetes.Storage.Location,
		)
		if err != nil {
			return "", errors.Wrap(err, "create bucket")
		}
	}

	buildFile := fmt.Sprintf("build-%s.tar.gz", b.ID)
	buildFilePath := fmt.Sprintf("%s/%s", b.stack.Namespace(), buildFile)

	_, err = b.storage.PutObject(
		kubernetes.Storage.Bucket, buildFilePath, b.Tarball, -1,
		minio.PutObjectOptions{
			ContentType: "application/gzip",
		},
	)

	return fmt.Sprintf(
		"%s/%s", kubernetes.Storage.Bucket, buildFilePath,
	), errors.Wrap(err, "put object")
}

func (b *Build) podName() string {
	return fmt.Sprintf("kaniko-%s-%s", b.config.Name, b.ID)
}

func (b *Build) kanikoDestination(registry, image string) string {
	return fmt.Sprintf(
		"%s/%s:%s", registry, image, b.ID,
	)
}

func (b *Build) Image(registry, image string) string {
	return b.kanikoDestination(registry, image)
}

func (b *Build) pod(
	buildTarballURL string,
) *corev1.Pod {
	var (
		kubernetes = b.config.Kubernetes
		storage    = kubernetes.Storage
	)

	configureMc := fmt.Sprintf(
		"mc config host add minio https://%s %s %s",
		storage.Endpoint, storage.AccessKey, storage.SecretKey,
	)

	downloadContext := fmt.Sprintf(
		"mc cp minio/%s /build/context.tar.gz", buildTarballURL,
	)

	return &corev1.Pod{
		Metadata: &metav1.ObjectMeta{
			Name:      k8s.String(b.podName()),
			Namespace: k8s.String(b.stack.Namespace()),
			Labels: map[string]string{
				"up-project":  b.config.Name,
				"up-stage":    b.Stage,
				"up-build-id": b.ID,
				"up-process":  "build",
			},
		},
		Spec: &corev1.PodSpec{
			InitContainers: []*corev1.Container{
				&corev1.Container{
					Name:    k8s.String("download-context"),
					Image:   k8s.String("minio/mc"),
					Command: []string{"/bin/sh"},
					Args: []string{
						"-c", fmt.Sprintf("%s && %s && mkdir /build/context && cd /build/context && tar xf ../context.tar.gz", configureMc, downloadContext),
					},
					VolumeMounts: []*corev1.VolumeMount{
						&corev1.VolumeMount{
							Name:      k8s.String("context"),
							MountPath: k8s.String("/build/"),
						},
					},
				},
			},
			Containers: []*corev1.Container{
				&corev1.Container{
					Name:  k8s.String(b.podName()),
					Image: k8s.String("gcr.io/kaniko-project/executor:latest"),
					Args: []string{
						// fmt.Sprintf("--dockerfile=%s", docker.Dockerfile),
						"--cache=true",
						fmt.Sprintf("--dockerfile=%s", "Dockerfile.up"),
						"--context=dir:///build/context",
						fmt.Sprintf("--destination=%s", b.kanikoDestination(kubernetes.Registry.URL, kubernetes.Registry.Image)),
					},
					Env: []*corev1.EnvVar{
						&corev1.EnvVar{
							Name:  k8s.String("AWS_SDK_LOAD_CONFIG"),
							Value: k8s.String("1"),
						},
					},
					VolumeMounts: []*corev1.VolumeMount{
						&corev1.VolumeMount{
							Name:      k8s.String("docker-config"),
							MountPath: k8s.String("/kaniko/.docker/"),
						},
						&corev1.VolumeMount{
							Name:      k8s.String("context"),
							MountPath: k8s.String("/build/"),
						},
					},
				},
			},
			RestartPolicy: k8s.String("Never"),
			Volumes: []*corev1.Volume{
				&corev1.Volume{
					Name: k8s.String("docker-config"),
					VolumeSource: &corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: k8s.String(stack.DockerRegistrySecret),
						},
					},
				},
				&corev1.Volume{
					Name: k8s.String("context"),
					VolumeSource: &corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: k8s.String(""),
						},
					},
				},
			},
		},
	}
}

func (b *Build) Run(ctx context.Context) error {
	if _, err := b.tarball(); err != nil {
		return errors.Wrap(err, "build tarball")
	}

	buildTarballURL, err := b.upload()
	if err != nil {
		return errors.Wrap(err, "upload context")
	}

	pod := b.pod(buildTarballURL)
	if err := b.k8s.Create(ctx, pod); err != nil {
		return errors.Wrap(err, "create pod")
	}

	label := &k8s.LabelSelector{}
	label.Eq("up-build-id", b.ID)
	label.Eq("up-process", "build")

	watcher, err := b.k8s.Watch(
		ctx, b.stack.Namespace(), pod, label.Selector(),
	)
	if err != nil {
		return errors.Wrap(err, "watch build")
	}
	defer watcher.Close()

	for {
		pod := new(corev1.Pod)
		_, err := watcher.Next(pod)
		if err != nil {
			return errors.Wrap(err, "watch next")
		}

		if *pod.Status.Phase == "Failed" {
			b.k8s.Delete(ctx, pod)
			watcher.Close()
			return errors.New("build failed")
		}

		if *pod.Status.Phase == "Succeeded" {
			b.k8s.Delete(ctx, pod)
			watcher.Close()
			break
		}
	}

	return nil
}

const runtimeDockerfile = `
FROM gliderlabs/herokuish:latest

ADD . /app
WORKDIR /app

RUN herokuish buildpack build

CMD ["/app/up-proxy"]
`

// injectProxy injects the Go proxy.
func (b *Build) injectProxy() error {
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
func (b *Build) removeProxy() error {
	log.Debugf("removing proxy")
	os.Remove("up-proxy")
	os.Remove("Dockerfile.up")
	return nil
}
