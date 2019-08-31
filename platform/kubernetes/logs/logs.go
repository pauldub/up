package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/apex/log"
	jsonlog "github.com/apex/log/handlers/json"
	"github.com/apex/up"
	"github.com/apex/up/internal/logs/text"
	"github.com/apex/up/internal/util"
	"github.com/apex/up/platform/kubernetes/stack"
	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	kcorev1 "k8s.io/api/core/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Logs struct {
	up.LogsConfig
	stack *stack.KubernetesStack
	w     io.WriteCloser
	io.Reader
}

func New(stack *stack.KubernetesStack, c up.LogsConfig) up.Logs {
	r, w := io.Pipe()

	l := &Logs{
		LogsConfig: c,
		stack:      stack,
		w:          w,
		Reader:     r,
	}
	go l.start()
	return l
}

func (l *Logs) start() {
	var (
		pods corev1.PodList
	)

	label := &k8s.LabelSelector{}
	label.Eq("up-project", l.stack.Config().Name)
	label.Eq("up-process", "deploy")

	err := l.stack.K8s().List(context.Background(), l.stack.Namespace(), &pods, label.Selector())
	if err != nil {
		return
	}

	readers := make([]io.Reader, 0)

	var sinceTime *kmetav1.Time
	zeroTime := time.Time{}

	if l.Since != zeroTime {
		sinceTime = &kmetav1.Time{
			Time: l.Since,
		}
	}

	for _, pod := range pods.Items {
		req := l.stack.Client().CoreV1().Pods(l.stack.Namespace()).GetLogs(*pod.Metadata.Name, &kcorev1.PodLogOptions{
			Follow:    l.Follow,
			SinceTime: sinceTime,
		})
		logs, err := req.Stream()

		if err != nil {
			return
		}
		defer logs.Close()

		readers = append(readers, logs)
	}

	var handler log.Handler

	if l.OutputJSON {
		handler = jsonlog.New(os.Stdout)
	} else {
		handler = text.New(os.Stdout).WithExpandedFields(l.Expand)
	}

	scanner := bufio.NewScanner(io.MultiReader(readers...))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// json log
		if util.IsJSONLog(line) {
			var e log.Entry
			err := json.Unmarshal([]byte(line), &e)
			if err != nil {
				log.Fatalf("error parsing json: %s", err)
			}

			handler.HandleLog(&e)
			continue
		}

		// lambda textual logs
		handler.HandleLog(&log.Entry{
			Level:   log.InfoLevel,
			Message: strings.TrimRight(line, " \n"),
		})
	}

	l.w.Close()
}
