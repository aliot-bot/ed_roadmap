package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *App) learningStore() (LearningStore, error) {
	s, ok := a.store.(LearningStore)
	if !ok {
		return nil, fmt.Errorf("learning sessions unavailable")
	}
	return s, nil
}

func (a *App) StartLearning(ctx context.Context, p domain.Principal, roadmap, element uuid.UUID) (domain.LearningSession, error) {
	if err := write(p); err != nil {
		return domain.LearningSession{}, err
	}
	s, err := a.learningStore()
	if err != nil {
		return domain.LearningSession{}, err
	}
	return s.StartLearningSession(ctx, p.UserID, roadmap, element, time.Now().UTC())
}

func (a *App) StartLearningElement(ctx context.Context, p domain.Principal, element uuid.UUID) (domain.LearningSession, error) {
	if err := read(p); err != nil {
		return domain.LearningSession{}, err
	}
	t, err := a.store.GetTopic(ctx, p.UserID, element)
	if err != nil {
		return domain.LearningSession{}, err
	}
	return a.StartLearning(ctx, p, t.RoadmapID, element)
}

func (a *App) ActiveLearning(ctx context.Context, p domain.Principal) (*domain.LearningSession, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	s, err := a.learningStore()
	if err != nil {
		return nil, err
	}
	// The filtered query is represented by a sentinel lookup in the postgres implementation.
	item, err := s.GetLearningSession(ctx, p.UserID, uuid.Nil)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (a *App) TransitionLearning(ctx context.Context, p domain.Principal, id uuid.UUID, status string, note *string) (domain.LearningSession, error) {
	if err := write(p); err != nil {
		return domain.LearningSession{}, err
	}
	s, err := a.learningStore()
	if err != nil {
		return domain.LearningSession{}, err
	}
	if status != domain.LearningPaused && status != domain.LearningActive && status != domain.LearningFinished {
		return domain.LearningSession{}, domain.ValidationError{Field: "status", Reason: "must be paused, active, or finished"}
	}
	return s.TransitionLearningSession(ctx, p.UserID, id, status, time.Now().UTC(), note)
}

func (a *App) LearningSessions(ctx context.Context, p domain.Principal, from, to time.Time) ([]domain.LearningSession, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	s, err := a.learningStore()
	if err != nil {
		return nil, err
	}
	return s.ListLearningSessions(ctx, p.UserID, from, to)
}

func (a *App) LearningOverview(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) (domain.LearningOverview, error) {
	items, err := a.LearningSessions(ctx, p, from, to)
	if err != nil {
		return domain.LearningOverview{}, err
	}
	if len(items) == 0 {
		return domain.LearningOverview{}, nil
	}
	total, topic, task := 0, 0, 0
	days := map[string]bool{}
	for _, item := range items {
		mins := int(item.DurationSeconds / 60)
		total += mins
		if item.Kind == domain.ElementTask {
			task += mins
		} else {
			topic += mins
		}
		days[item.StartedAt.In(loc).Format("2006-01-02")] = true
	}
	return domain.LearningOverview{StudyMinutes: &total, TopicMinutes: &topic, TaskMinutes: &task, ActiveDays: ptrInt(len(days))}, nil
}

func (a *App) LearningActivity(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) ([]domain.LearningActivityDay, error) {
	items, err := a.LearningSessions(ctx, p, from, to)
	if err != nil {
		return nil, err
	}
	byDaySeconds := map[string]int64{}
	if segStore, ok := a.store.(LearningSegmentsStore); ok {
		segments, e := segStore.ListLearningSegments(ctx, p.UserID, from, to)
		if e != nil {
			return nil, e
		}
		for _, seg := range segments {
			end := time.Now().UTC()
			if seg.EndedAt != nil {
				end = *seg.EndedAt
			}
			if end.Before(from) || !seg.StartedAt.Before(to) {
				continue
			}
			start := seg.StartedAt
			if start.Before(from) {
				start = from
			}
			if end.After(to) {
				end = to
			}
			for start.Before(end) {
				next := end
				localNext := start.In(loc).AddDate(0, 0, 1)
				boundary := time.Date(localNext.Year(), localNext.Month(), localNext.Day(), 0, 0, 0, 0, loc).In(time.UTC)
				if boundary.Before(next) {
					next = boundary
				}
				seconds := int64(next.Sub(start).Seconds())
				if seconds > 0 {
					byDaySeconds[start.In(loc).Format("2006-01-02")] += seconds
				}
				start = next
			}
		}
	} else {
		for _, item := range items {
			byDaySeconds[item.StartedAt.In(loc).Format("2006-01-02")] += item.DurationSeconds
		}
	}
	out := make([]domain.LearningActivityDay, 0, len(byDaySeconds))
	for date, seconds := range byDaySeconds {
		minutes := int(seconds / 60)
		level := "low"
		if minutes == 0 {
			level = "none"
		} else if minutes >= 120 {
			level = "max"
		} else if minutes >= 60 {
			level = "high"
		} else if minutes >= 30 {
			level = "moderate"
		}
		ach := []string{}
		if minutes >= 240 {
			ach = append(ach, "four_hours")
		}
		if minutes >= 360 {
			ach = append(ach, "six_hours")
		}
		if minutes >= 480 {
			ach = append(ach, "eight_hours")
		}
		out = append(out, domain.LearningActivityDay{Date: date, StudyMinutes: minutes, Intensity: level, Achievements: ach})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func (a *App) LearningAchievements(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) (domain.LearningAchievements, error) {
	days, err := a.LearningActivity(ctx, p, from, to, loc)
	if err != nil {
		return domain.LearningAchievements{}, err
	}
	out := domain.LearningAchievements{}
	for _, d := range days {
		for _, x := range d.Achievements {
			switch x {
			case "four_hours":
				out.FourHours++
			case "six_hours":
				out.SixHours++
			case "eight_hours":
				out.EightHours++
			}
		}
	}
	return out, nil
}

func activityIntensity(minutes int) (string, []string) {
	level := "low"
	if minutes == 0 {
		level = "none"
	} else if minutes >= 120 {
		level = "max"
	} else if minutes >= 60 {
		level = "high"
	} else if minutes >= 30 {
		level = "moderate"
	}
	ach := []string{}
	if minutes >= 240 {
		ach = append(ach, "four_hours")
	}
	if minutes >= 360 {
		ach = append(ach, "six_hours")
	}
	if minutes >= 480 {
		ach = append(ach, "eight_hours")
	}
	return level, ach
}
