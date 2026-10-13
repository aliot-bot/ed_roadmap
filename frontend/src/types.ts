export interface User {
  id: string
  login: string
  display_name?: string
}

export interface Roadmap {
  id: string
  title: string
  description: string
  created_at?: string
  updated_at?: string
  has_cover?: boolean
  progress?: Progress
}

export interface Topic {
  id: string
  roadmap_id: string
  parent_id: string | null
  title: string
  description: string
  sort_order: number
  completed: boolean
  status?: TopicStatus
  priority?: TopicPriority
  note?: string
  kind?: 'topic' | 'task'
  task_status?: 'not_started' | 'in_progress' | 'completed'
}

export type TopicStatus = 'not_started' | 'in_progress' | 'needs_review' | 'completed'
export type TopicPriority = 'critical' | 'important' | 'optional' | 'skippable'

export interface LearningSession { id: string; element_id: string; roadmap_id?: string; started_at: string; updated_at?: string; paused_at?: string; finished_at?: string; elapsed_seconds?: number; status: 'active' | 'paused' | 'finished' }
export interface LearningOverview { study_minutes: number; topic_minutes: number; task_minutes: number; active_days: number; active?: LearningSession }
export interface LearningActivity { date: string; study_minutes: number }
export interface LearningAchievement { kind: '4h' | '6h' | '8h'; days: number }

export interface Dependency {
  predecessor_topic_id: string
  successor_topic_id: string
}

export interface Progress {
  completed: number
  total: number
  percent: number | null
  status_counts?: { not_started: number; in_progress: number; needs_review: number; completed: number }
}

export interface TopicLayout {
  topic_id: string
  x: number
  y: number
  collapsed?: boolean
}

export interface TopicDocument {
  content: string
  updated_at?: string
}

export interface ChecklistItem {
  id: string
  topic_id: string
  text: string
  completed: boolean
  sort_order: number
}

export interface ChecklistItemInput {
  id?: string
  text: string
  completed: boolean
  sort_order: number
}

export interface RoadmapAnalyticsItem {
  roadmap_id: string
  title: string
  total: number
  completed: number
  remaining: number
  percent: number
}

export interface Analytics {
  roadmap_count?: number
  total: number
  completed: number
  remaining: number
  percent: number
  roadmaps: RoadmapAnalyticsItem[]
}

export interface RoadmapData {
  roadmap: Roadmap
  topics: Topic[]
  dependencies: Dependency[]
  progress: Progress
  layout: TopicLayout[]
}
