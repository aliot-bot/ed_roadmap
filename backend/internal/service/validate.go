package service

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func ValidateHierarchy(topics []domain.Topic, target uuid.UUID, parent *uuid.UUID) error {
	if parent == nil {
		return nil
	}
	if *parent == target {
		return domain.ValidationError{Field: "parent_id", Reason: "topic cannot be its own parent"}
	}
	parents := make(map[uuid.UUID]*uuid.UUID, len(topics))
	exists := false
	for i := range topics {
		parents[topics[i].ID] = topics[i].ParentID
		if topics[i].ID == *parent {
			exists = true
		}
	}
	if !exists {
		return domain.ValidationError{Field: "parent_id", Reason: "parent must belong to the same roadmap"}
	}
	seen := map[uuid.UUID]bool{target: true}
	current := parent
	for current != nil {
		if seen[*current] {
			return domain.ValidationError{Field: "parent_id", Reason: "hierarchy cycle detected"}
		}
		seen[*current] = true
		current = parents[*current]
	}
	return nil
}

func ValidateDependencies(topicIDs []uuid.UUID, deps []domain.Dependency) error {
	allowed := make(map[uuid.UUID]bool, len(topicIDs))
	for _, id := range topicIDs {
		allowed[id] = true
	}
	graph := make(map[uuid.UUID][]uuid.UUID)
	unique := map[string]bool{}
	for i, d := range deps {
		if d.PredecessorTopicID == d.SuccessorTopicID {
			return domain.ValidationError{Field: fmt.Sprintf("dependencies[%d]", i), Reason: "self dependency is not allowed"}
		}
		if !allowed[d.PredecessorTopicID] || !allowed[d.SuccessorTopicID] {
			return domain.ValidationError{Field: fmt.Sprintf("dependencies[%d]", i), Reason: "both topics must belong to the roadmap"}
		}
		key := d.PredecessorTopicID.String() + ":" + d.SuccessorTopicID.String()
		if unique[key] {
			return domain.ValidationError{Field: fmt.Sprintf("dependencies[%d]", i), Reason: "duplicate dependency"}
		}
		unique[key] = true
		graph[d.PredecessorTopicID] = append(graph[d.PredecessorTopicID], d.SuccessorTopicID)
	}
	state := map[uuid.UUID]uint8{}
	var visit func(uuid.UUID) bool
	visit = func(id uuid.UUID) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, n := range graph[id] {
			if visit(n) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for _, id := range topicIDs {
		if visit(id) {
			return domain.ValidationError{Field: "dependencies", Reason: "dependency cycle detected"}
		}
	}
	return nil
}

func CalculateProgress(topics []domain.Topic) domain.Progress {
	children := make(map[uuid.UUID][]uuid.UUID)
	byID := make(map[uuid.UUID]domain.Topic, len(topics))
	for _, t := range topics {
		byID[t.ID] = t
		if t.ParentID != nil {
			children[*t.ParentID] = append(children[*t.ParentID], t.ID)
		}
	}
	leafStats := func(root *uuid.UUID) (int, int) {
		total, done := 0, 0
		for _, t := range topics {
			if len(children[t.ID]) != 0 || t.Kind == domain.ElementTask {
				continue
			}
			include := root == nil
			if root != nil {
				cur := t.ParentID
				for cur != nil {
					if *cur == *root {
						include = true
						break
					}
					p := byID[*cur]
					cur = p.ParentID
				}
			}
			if include {
				total++
				if t.Completed {
					done++
				}
			}
		}
		return total, done
	}
	total, done := leafStats(nil)
	out := domain.Progress{Total: total, Completed: done, StatusCounts: map[string]int{domain.TopicNotStarted: 0, domain.TopicInProgress: 0, domain.TopicNeedsReview: 0, domain.TopicCompleted: 0}}
	for _, t := range topics {
		if len(children[t.ID]) == 0 && t.Kind != domain.ElementTask {
			status := t.Status
			if status == "" {
				if t.Completed {
					status = domain.TopicCompleted
				} else {
					status = domain.TopicNotStarted
				}
			}
			out.StatusCounts[status]++
		}
	}
	if total > 0 {
		v := float64(done) * 100 / float64(total)
		out.Percent = &v
	}
	for _, t := range topics {
		if len(children[t.ID]) == 0 {
			continue
		}
		n, d := leafStats(&t.ID)
		g := domain.GroupProgress{TopicID: t.ID, Total: n, Completed: d}
		if n > 0 {
			v := float64(d) * 100 / float64(n)
			g.Percent = &v
		}
		out.Groups = append(out.Groups, g)
	}
	return out
}

func validateText(field, value string, min, max int) error {
	n := len([]rune(strings.TrimSpace(value)))
	if n < min || n > max {
		return domain.ValidationError{Field: field, Reason: fmt.Sprintf("must contain %d-%d characters", min, max)}
	}
	return nil
}
