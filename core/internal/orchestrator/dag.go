package orchestrator

import (
	"fmt"

	"github.com/kienbm/magic-claw/core/internal/protocol"
)

func FindReadySteps(steps []protocol.WorkflowStep) []string {
	resolved := make(map[string]bool)
	for _, s := range steps {
		if s.Status == protocol.StepCompleted || s.Status == protocol.StepSkipped {
			resolved[s.ID] = true
		}
	}

	var ready []string
	for _, s := range steps {
		if s.Status != protocol.StepPending {
			continue
		}
		allDepsOK := true
		for _, dep := range s.DependsOn {
			if !resolved[dep] {
				allDepsOK = false
				break
			}
		}
		if allDepsOK {
			ready = append(ready, s.ID)
		}
	}
	return ready
}

func IsWorkflowDone(steps []protocol.WorkflowStep) bool {
	for _, s := range steps {
		switch s.Status {
		case protocol.StepCompleted, protocol.StepSkipped, protocol.StepFailed:
			continue
		default:
			return false
		}
	}
	return true
}

func HasFailed(steps []protocol.WorkflowStep) bool {
	for _, s := range steps {
		if s.Status == protocol.StepFailed {
			return true
		}
	}
	return false
}

func ValidateDAG(steps []protocol.WorkflowStep) error {
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for _, s := range steps {
		inDegree[s.ID] = len(s.DependsOn)
		for _, dep := range s.DependsOn {
			dependents[dep] = append(dependents[dep], s.ID)
		}
	}

	var queue []string
	for _, s := range steps {
		if inDegree[s.ID] == 0 {
			queue = append(queue, s.ID)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if visited != len(steps) {
		return fmt.Errorf("workflow contains a cycle")
	}
	return nil
}
