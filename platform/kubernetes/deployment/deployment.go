package deployment

import (
	"context"
	"errors"

	"github.com/apex/up"
	"github.com/apex/up/platform/event"
	"github.com/apex/up/platform/kubernetes/build"
	"github.com/apex/up/platform/kubernetes/stack"
)

type Deployment struct {
	stack  stack.Stack
	build  *build.Build
	events event.Events
	info   up.Deploy
}

func New(
	stack stack.Stack, build *build.Build,
	events event.Events, deploy up.Deploy,
) *Deployment {
	return &Deployment{
		stack:  stack,
		build:  build,
		events: events,
		info:   deploy,
	}
}

func (d *Deployment) Deploy(ctx context.Context) error {
	return errors.New("not implemented")
}
