package resolver

import (
	"context"
	"errors"
	"sort"

	"github.com/nabhold/baobab-cp/internal/domain"
)

// EngineInstance is the runtime engine instance selected by the topology resolver.
type EngineInstance = domain.EngineInstance

// TopologyResolutionQuery resolves an engine instance within the current trusted context.
type TopologyResolutionQuery struct {
	Context         Context
	EngineInstances []EngineInstance
}

// TopologyResolverImpl resolves an engine instance to the highest-ranked active candidate.
type TopologyResolverImpl struct{}

func (TopologyResolverImpl) Resolve(_ context.Context, q TopologyResolutionQuery) (EngineInstance, error) {
	if len(q.EngineInstances) == 0 {
		return EngineInstance{}, errors.New("engine instance not found")
	}

	active := make([]EngineInstance, 0, len(q.EngineInstances))
	for _, instance := range q.EngineInstances {
		if instance.Status != "ACTIVE" {
			continue
		}
		if instance.Environment == "" {
			continue
		}
		active = append(active, instance)
	}
	if len(active) == 0 {
		return EngineInstance{}, errors.New("engine instance not found")
	}

	sort.Slice(active, func(i, j int) bool {
		if active[i].Environment != active[j].Environment {
			return active[i].Environment == "production"
		}
		if active[i].Region != active[j].Region {
			return active[i].Region < active[j].Region
		}
		return active[i].ID < active[j].ID
	})

	return active[0], nil
}
