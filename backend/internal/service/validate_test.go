package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func TestValidateHierarchyRejectsCyclesAndForeignParent(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	topics := []domain.Topic{{ID: a}, {ID: b, ParentID: &a}}
	if err := ValidateHierarchy(topics, a, &b); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("cycle: got %v", err)
	}
	if err := ValidateHierarchy(topics, a, ptr(uuid.New())); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("foreign parent: got %v", err)
	}
	if err := ValidateHierarchy(topics, a, nil); err != nil {
		t.Fatalf("root parent: %v", err)
	}
}

func TestValidateTopicKind(t *testing.T) {
	if err := validateTopicKind(domain.ElementTask, domain.TaskInProgress); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
	if err := validateTopicKind("unknown", domain.TaskNotStarted); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("unknown kind accepted")
	}
	if err := validateTopicKind(domain.ElementTopic, domain.TaskCompleted); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("topic task status accepted")
	}
}

func TestValidateDependenciesRejectsCycleSelfAndForeign(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	ids := []uuid.UUID{a, b}
	for _, deps := range [][]domain.Dependency{
		{{PredecessorTopicID: a, SuccessorTopicID: a}},
		{{PredecessorTopicID: a, SuccessorTopicID: b}, {PredecessorTopicID: b, SuccessorTopicID: a}},
		{{PredecessorTopicID: a, SuccessorTopicID: c}},
	} {
		if !errors.Is(ValidateDependencies(ids, deps), domain.ErrValidation) {
			t.Fatalf("deps %#v accepted", deps)
		}
	}
	if err := ValidateDependencies(ids, []domain.Dependency{{PredecessorTopicID: a, SuccessorTopicID: b}}); err != nil {
		t.Fatalf("valid DAG rejected: %v", err)
	}
}

func TestCalculateProgressCountsLeavesOnly(t *testing.T) {
	group, leafDone, leafOpen, nested := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	topics := []domain.Topic{
		{ID: group, Completed: true},
		{ID: leafDone, ParentID: &group, Completed: true},
		{ID: leafOpen, ParentID: &group},
		{ID: nested, ParentID: &leafOpen, Completed: true},
	}
	p := CalculateProgress(topics)
	if p.Total != 2 || p.Completed != 2 || p.Percent == nil || *p.Percent != 100 {
		t.Fatalf("unexpected roadmap progress: %#v", p)
	}
	if len(p.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(p.Groups))
	}
	for _, g := range p.Groups {
		if g.TopicID == group && (g.Total != 2 || g.Completed != 2) {
			t.Fatalf("group progress: %#v", g)
		}
	}
}

func TestCalculateProgressReportsLeafStatusCounts(t *testing.T) {
	parent := uuid.New()
	topics := []domain.Topic{{ID: uuid.New(), Status: domain.TopicNotStarted}, {ID: uuid.New(), Status: domain.TopicInProgress}, {ID: uuid.New(), Status: domain.TopicNeedsReview}, {ID: uuid.New(), Status: domain.TopicCompleted, Completed: true}, {ID: parent}, {ID: uuid.New(), Status: domain.TopicNeedsReview, ParentID: &parent}}
	p := CalculateProgress(topics)
	if p.StatusCounts[domain.TopicNotStarted] != 1 || p.StatusCounts[domain.TopicInProgress] != 1 || p.StatusCounts[domain.TopicNeedsReview] != 2 || p.StatusCounts[domain.TopicCompleted] != 1 {
		t.Fatalf("status counts=%v", p.StatusCounts)
	}
}

func TestCalculateProgressExcludesTaskLeaves(t *testing.T) {
	topics := []domain.Topic{{ID: uuid.New(), Kind: domain.ElementTopic, Completed: true, Status: domain.TopicCompleted}, {ID: uuid.New(), Kind: domain.ElementTask, TaskStatus: domain.TaskCompleted}}
	p := CalculateProgress(topics)
	if p.Total != 1 || p.Completed != 1 {
		t.Fatalf("tasks included in progress: %#v", p)
	}
}

func TestActivityAchievementBoundariesAreCumulative(t *testing.T) {
	for _, tc := range []struct {
		minutes int
		want    int
	}{{239, 0}, {240, 1}, {359, 1}, {360, 2}, {479, 2}, {480, 3}} {
		_, got := activityIntensity(tc.minutes)
		if len(got) != tc.want {
			t.Fatalf("%d minutes: got %v", tc.minutes, got)
		}
	}
}

func TestValidateHierarchyHandlesDeepTreeAndCycleDiscoveredAtTheEnd(t *testing.T) {
	const depth = 750
	ids := make([]uuid.UUID, depth)
	topics := make([]domain.Topic, depth)
	for i := range ids {
		ids[i] = uuid.New()
		topics[i] = domain.Topic{ID: ids[i]}
		if i > 0 {
			topics[i].ParentID = ptr(ids[i-1])
		}
	}

	// A new topic can be attached to the deepest node in a large, valid tree.
	if err := ValidateHierarchy(topics, uuid.New(), &ids[depth-1]); err != nil {
		t.Fatalf("deep valid hierarchy rejected: %v", err)
	}

	// Simulate moving the root below the deepest node. The invalid edge is
	// discovered only after walking hundreds of ancestors.
	topics[depth-1].ParentID = ptr(ids[depth/2])
	if err := ValidateHierarchy(topics, ids[0], &ids[depth-1]); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("late hierarchy cycle accepted: %v", err)
	}
}

func TestValidateDependenciesHandlesLongDAGAndLateCycle(t *testing.T) {
	const depth = 1200
	ids := make([]uuid.UUID, depth)
	deps := make([]domain.Dependency, 0, depth)
	for i := range ids {
		ids[i] = uuid.New()
		if i > 0 {
			deps = append(deps, domain.Dependency{PredecessorTopicID: ids[i-1], SuccessorTopicID: ids[i]})
		}
	}
	if err := ValidateDependencies(ids, deps); err != nil {
		t.Fatalf("long DAG rejected: %v", err)
	}

	// The back edge is last, so the cycle is found after traversing the long
	// chain rather than at its beginning.
	deps = append(deps, domain.Dependency{PredecessorTopicID: ids[depth-1], SuccessorTopicID: ids[depth/3]})
	if err := ValidateDependencies(ids, deps); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("late dependency cycle accepted: %v", err)
	}
}

func TestCalculateProgressComplexTreeCountsOnlyLeaves(t *testing.T) {
	rootA, rootB := uuid.New(), uuid.New()
	topics := []domain.Topic{{ID: rootA, Completed: true}, {ID: rootB, Completed: true}}
	for i := 0; i < 260; i++ {
		topics = append(topics, domain.Topic{ID: uuid.New(), ParentID: ptr(rootA), Completed: i%3 == 0})
	}
	for i := 0; i < 180; i++ {
		topics = append(topics, domain.Topic{ID: uuid.New(), ParentID: ptr(rootB), Completed: i%4 < 2})
	}

	// Add a deep branch with one completed leaf. Every internal node is marked
	// completed deliberately; progress must still be based on its leaf only.
	parent := rootA
	const chainLength = 140
	for i := 0; i < chainLength; i++ {
		next := uuid.New()
		topics = append(topics, domain.Topic{ID: next, ParentID: ptr(parent), Completed: true})
		parent = next
	}
	topics = append(topics, domain.Topic{ID: uuid.New(), ParentID: ptr(parent), Completed: true})

	p := CalculateProgress(topics)
	if p.Total != 441 || p.Completed != 178 || p.Percent == nil || *p.Percent != 178*100.0/441 {
		t.Fatalf("unexpected complex roadmap progress: %#v", p)
	}
	byID := make(map[uuid.UUID]domain.GroupProgress, len(p.Groups))
	for _, group := range p.Groups {
		byID[group.TopicID] = group
	}
	if got := byID[rootA]; got.Total != 261 || got.Completed != 88 || got.Percent == nil || *got.Percent != 88*100.0/261 {
		t.Fatalf("root A progress counted non-leaves: %#v", got)
	}
	if got := byID[rootB]; got.Total != 180 || got.Completed != 90 || got.Percent == nil || *got.Percent != 50 {
		t.Fatalf("root B progress: %#v", got)
	}
	if len(p.Groups) != chainLength+2 {
		t.Fatalf("groups = %d, want %d", len(p.Groups), chainLength+2)
	}
	for _, topic := range topics {
		if topic.ID == rootA || topic.ID == rootB || topic.ParentID == nil {
			continue
		}
		// All chain internal nodes have exactly one completed leaf. The broad
		// leaves do not appear in Groups because they are leaves themselves.
		if _, ok := byID[topic.ID]; ok && topic.Completed {
			if got := byID[topic.ID]; got.Total != 1 || got.Completed != 1 || got.Percent == nil || *got.Percent != 100 {
				t.Fatalf("deep group progress: %#v", got)
			}
		}
	}
}

func ptr(v uuid.UUID) *uuid.UUID { return &v }
