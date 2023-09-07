package probe

import (
	"context"
	"errors"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	ErrProbeNotExists     = errors.New("probe not exists")
	ErrProbeAlreadyExists = errors.New("probe already exists")
	ErrInvalidProbeState  = errors.New("invalid probe state")
)

type Type uint8

type EventType string

const (
	ProbeTypeMetrics = iota
	ProbeTypeEvent
	ProbeTypeCount
)

func (p Type) String() string {
	switch p {
	case ProbeTypeMetrics:
		return "metrics"
	case ProbeTypeEvent:
		return "event"
	default:
		return ""
	}
}

type State uint8

const (
	ProbeStateStopped = iota
	ProbeStateStarting
	ProbeStateRunning
	ProbeStateStopping
	ProbeStateFailed
)

func (ps State) String() string {
	switch ps {
	case ProbeStateStopped:
		return "Stopped"
	case ProbeStateRunning:
		return "Running"
	case ProbeStateStarting:
		return "Starting"
	case ProbeStateStopping:
		return "Stopping"
	}
	return ""
}

type RawEvent struct {
	Netns     uint32
	EventType string
	EventBody string
}

type Label struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Event struct {
	Timestamp int64     `json:"timestamp"`
	Type      EventType `json:"type"`
	Labels    []Label   `json:"labels"`
	Message   string    `json:"msg"`
}

type Probe interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	State() State
	Name() string
}

type MetricsProbe interface {
	Probe
	prometheus.Collector
}

type EventProbe interface {
	Probe
}

type SimpleProbe interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type simpleProbe struct {
	name  string
	state State
	inner SimpleProbe
	lock  sync.Mutex
}

func (s *simpleProbe) Start(ctx context.Context) error {
	if s.state != ProbeStateStopped {
		return ErrInvalidProbeState
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.state = ProbeStateStarting
	if err := s.inner.Start(ctx); err != nil {
		s.state = ProbeStateFailed
		return err
	}
	s.state = ProbeStateRunning
	return nil
}

func (s *simpleProbe) Stop(ctx context.Context) error {
	if s.state != ProbeStateRunning {
		return ErrInvalidProbeState
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	if err := s.inner.Stop(ctx); err != nil {
		s.state = ProbeStateFailed
		return err
	}
	s.state = ProbeStateStopped
	return nil
}

func (s *simpleProbe) State() State {
	return s.state
}

func (s *simpleProbe) Name() string {
	return s.name
}

func NewProbe(name string, probe SimpleProbe) Probe {
	return &simpleProbe{
		name:  name,
		inner: probe,
	}
}
