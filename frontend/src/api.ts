import type { Analytics, ChecklistItem, ChecklistItemInput, Dependency, LearningActivity, LearningAchievement, LearningOverview, LearningSession, Progress, Roadmap, RoadmapAnalyticsItem, Topic, TopicDocument, TopicLayout, User, TopicPriority, TopicStatus } from './types'

const API_ROOT = import.meta.env.VITE_API_URL ?? '/api'

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly details?: unknown,
  ) {
    super(message)
  }
}

let csrfToken = ''

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_ROOT}${path}`, {
    credentials: 'include',
    ...options,
    headers: {
      ...(options.body && !(options.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}),
      ...(csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
      ...options.headers,
    },
  })
  const contentType = response.headers.get('content-type') ?? ''
  const body: unknown = contentType.includes('application/json') ? await response.json() : await response.text()
  const headerToken = response.headers.get('x-csrf-token')
  if (headerToken) csrfToken = headerToken
  if (!response.ok) {
    const record = asRecord(body)
    const message = stringValue(record?.message) || stringValue(record?.error) || friendlyStatus(response.status)
    throw new ApiError(message, response.status, body)
  }
  return body as T
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : undefined
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function numberValue(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function friendlyStatus(status: number): string {
  if (status === 401) return 'Войдите, чтобы продолжить'
  if (status === 403) return 'Недостаточно прав для этого действия'
  if (status === 404) return 'Объект не найден'
  if (status === 409) return 'Изменение конфликтует с текущей структурой карты'
  if (status === 429) return 'Слишком много попыток. Попробуйте немного позже'
  return 'Не удалось выполнить запрос'
}

function items<T>(value: unknown): T[] {
  if (Array.isArray(value)) return value as T[]
  const record = asRecord(value)
  return (Array.isArray(record?.items) ? record.items : []) as T[]
}

function roadmap(value: unknown): Roadmap {
  const raw = asRecord(value) ?? {}
  return {
    id: stringValue(raw.id),
    title: stringValue(raw.title),
    description: stringValue(raw.description),
    created_at: stringValue(raw.created_at) || undefined,
    updated_at: stringValue(raw.updated_at) || undefined,
    has_cover: raw.has_cover === true,
    progress: raw.progress ? progress(raw.progress) : undefined,
  }
}

function topic(value: unknown): Topic {
  const raw = asRecord(value) ?? {}
  const status = raw.status === 'in_progress' || raw.status === 'needs_review' || raw.status === 'completed' ? raw.status as TopicStatus : raw.completed === true ? 'completed' : 'not_started'
  return {
    id: stringValue(raw.id),
    roadmap_id: stringValue(raw.roadmap_id),
    parent_id: stringValue(raw.parent_id) || null,
    title: stringValue(raw.title),
    description: stringValue(raw.description),
    sort_order: numberValue(raw.sort_order),
    completed: raw.completed === true,
    status,
    priority: raw.priority === 'critical' || raw.priority === 'optional' || raw.priority === 'skippable' ? raw.priority as TopicPriority : 'important',
    note: stringValue(raw.note),
    kind: raw.kind === 'task' ? 'task' : 'topic',
    task_status: raw.task_status === 'in_progress' || raw.task_status === 'completed' ? raw.task_status : 'not_started',
  }
}

function dependency(value: unknown): Dependency {
  const raw = asRecord(value) ?? {}
  return {
    predecessor_topic_id: stringValue(raw.predecessor_topic_id),
    successor_topic_id: stringValue(raw.successor_topic_id),
  }
}

function progress(value: unknown): Progress {
  const raw = asRecord(value) ?? {}
  const completed = numberValue(raw.completed, numberValue(raw.completed_count))
  const total = numberValue(raw.total, numberValue(raw.total_count))
  const rawPercent = raw.percent ?? raw.percentage
  return {
    completed,
    total,
    percent: total === 0 ? null : numberValue(rawPercent, Math.round((completed / total) * 100)),
    status_counts: {
      not_started: numberValue(asRecord(raw.status_counts)?.not_started),
      in_progress: numberValue(asRecord(raw.status_counts)?.in_progress),
      needs_review: numberValue(asRecord(raw.status_counts)?.needs_review),
      completed: numberValue(asRecord(raw.status_counts)?.completed, completed),
    },
  }
}

function layoutItem(value: unknown): TopicLayout {
  const raw = asRecord(value) ?? {}
  return {
    topic_id: stringValue(raw.topic_id),
    x: numberValue(raw.x),
    y: numberValue(raw.y),
    collapsed: raw.collapsed === true,
  }
}

function document(value: unknown): TopicDocument {
  const raw = asRecord(value) ?? {}
  return { content: stringValue(raw.content), updated_at: stringValue(raw.updated_at) || undefined }
}
function checklistItem(value: unknown): ChecklistItem {
  const raw = asRecord(value) ?? {}
  return { id: stringValue(raw.id), topic_id: stringValue(raw.topic_id), text: stringValue(raw.text), completed: raw.completed === true, sort_order: numberValue(raw.sort_order) }
}

function learningSession(value: unknown): LearningSession | null {
  const raw = asRecord(asRecord(value)?.session ?? value)
  if (!raw || !stringValue(raw.id)) return null
  return {
    id: stringValue(raw.id),
    element_id: stringValue(raw.element_id),
    roadmap_id: stringValue(raw.roadmap_id) || undefined,
    started_at: stringValue(raw.started_at),
    updated_at: stringValue(raw.updated_at) || undefined,
    finished_at: stringValue(raw.finished_at) || undefined,
    elapsed_seconds: numberValue(raw.duration_seconds, numberValue(raw.elapsed_seconds)),
    status: raw.status === 'paused' || raw.status === 'finished' ? raw.status : 'active',
  }
}

function learningReport(value: unknown): { markdown?: string; updated_at?: string; status?: string; reason?: string } {
  const raw = asRecord(asRecord(value)?.report ?? value) ?? {}
  return {
    markdown: stringValue(raw.markdown) || undefined,
    updated_at: stringValue(raw.created_at ?? raw.updated_at) || undefined,
    status: stringValue(raw.status) || undefined,
    reason: stringValue(raw.reason) || undefined,
  }
}

function analyticsItem(value: unknown): RoadmapAnalyticsItem {
  const raw = asRecord(value) ?? {}
  const leaf = asRecord(raw.leaf_topics) ?? raw
  const total = numberValue(leaf.total, numberValue(leaf.total_count))
  const completed = numberValue(leaf.completed, numberValue(leaf.completed_count))
  return {
    roadmap_id: stringValue(raw.roadmap_id ?? raw.id),
    title: stringValue(raw.title ?? raw.name),
    total,
    completed,
    remaining: numberValue(leaf.remaining, Math.max(0, total - completed)),
    percent: numberValue(leaf.percent, total ? Math.round((completed / total) * 100) : 0),
  }
}

function analytics(value: unknown): Analytics {
  const raw = asRecord(value) ?? {}
  const leaf = asRecord(raw.leaf_topics) ?? raw
  const total = numberValue(leaf.total, numberValue(leaf.total_count))
  const completed = numberValue(leaf.completed, numberValue(leaf.completed_count))
  const rawRoadmaps = Array.isArray(raw.roadmaps) ? raw.roadmaps : Array.isArray(raw.items) ? raw.items : []
  return {
    roadmap_count: numberValue(raw.roadmap_count, rawRoadmaps.length),
    total,
    completed,
    remaining: numberValue(leaf.remaining, Math.max(0, total - completed)),
    percent: numberValue(leaf.percent, total ? Math.round((completed / total) * 100) : 0),
    roadmaps: rawRoadmaps.map(analyticsItem),
  }
}

export const api = {
  async me(): Promise<User> {
    const body = await request<unknown>('/auth/me')
    const raw = asRecord(asRecord(body)?.user ?? body) ?? {}
    return { id: stringValue(raw.id), login: stringValue(raw.login ?? raw.email), display_name: stringValue(raw.display_name) || undefined }
  },
  async login(login: string, password: string): Promise<User> {
    const body = await request<unknown>('/auth/login', { method: 'POST', body: JSON.stringify({ login, password }) })
    const raw = asRecord(asRecord(body)?.user ?? body) ?? {}
    return { id: stringValue(raw.id), login: stringValue(raw.login ?? raw.email) || login }
  },
  async keyLogin(key: string): Promise<User> {
    const body = await request<unknown>('/auth/key-login', { method: 'POST', body: JSON.stringify({ key }) })
    const raw = asRecord(asRecord(body)?.user ?? body) ?? {}
    return { id: stringValue(raw.id), login: stringValue(raw.login ?? 'Владелец') }
  },
  async bootstrap(login: string, password: string): Promise<User> {
    const body = await request<unknown>('/auth/bootstrap', { method: 'POST', body: JSON.stringify({ login, password }) })
    const raw = asRecord(asRecord(body)?.user ?? body) ?? {}
    return { id: stringValue(raw.id), login: stringValue(raw.login ?? raw.email) || login }
  },
  logout: () => request<void>('/auth/logout', { method: 'POST' }),

  async listRoadmaps(): Promise<Roadmap[]> {
    return items<unknown>(await request('/roadmaps')).map(roadmap)
  },
  async getRoadmap(id: string): Promise<Roadmap> {
    const body = await request<unknown>(`/roadmaps/${id}`)
    return roadmap(asRecord(body)?.roadmap ?? body)
  },
  async createRoadmap(input: Pick<Roadmap, 'title' | 'description'>): Promise<Roadmap> {
    return roadmap(await request('/roadmaps', { method: 'POST', body: JSON.stringify(input) }))
  },
  async updateRoadmap(id: string, input: Partial<Pick<Roadmap, 'title' | 'description'>>): Promise<Roadmap> {
    return roadmap(await request(`/roadmaps/${id}`, { method: 'PATCH', body: JSON.stringify(input) }))
  },
  deleteRoadmap: (id: string) => request<void>(`/roadmaps/${id}`, { method: 'DELETE' }),
  uploadRoadmapCover: (id: string, file: File) => {
    const body = new FormData()
    body.append('file', file)
    return request<{ mime_type: string; updated_at: string }>(`/roadmaps/${id}/cover`, { method: 'PUT', body })
  },
  deleteRoadmapCover: (id: string) => request<void>(`/roadmaps/${id}/cover`, { method: 'DELETE' }),

  async listTopics(roadmapId: string): Promise<Topic[]> {
    return items<unknown>(await request(`/roadmaps/${roadmapId}/topics`)).map(topic)
  },
  async createTopic(roadmapId: string, input: Pick<Topic, 'title' | 'description' | 'parent_id' | 'sort_order' | 'kind' | 'task_status' | 'status' | 'priority'>): Promise<Topic> {
    return topic(await request(`/roadmaps/${roadmapId}/topics`, { method: 'POST', body: JSON.stringify(input) }))
  },
  async updateTopic(id: string, input: Partial<Pick<Topic, 'title' | 'description' | 'parent_id' | 'sort_order' | 'kind' | 'task_status' | 'status' | 'priority'>>): Promise<Topic> {
    return topic(await request(`/topics/${id}`, { method: 'PATCH', body: JSON.stringify(input) }))
  },
  deleteTopic: (id: string) => request<void>(`/topics/${id}`, { method: 'DELETE' }),
  setCompletion: (id: string, completed: boolean) => request<unknown>(`/topics/${id}/completion`, { method: 'PUT', body: JSON.stringify({ completed }) }),
  setStatus: (id: string, status: TopicStatus) => request<unknown>(`/topics/${id}/status`, { method: 'PUT', body: JSON.stringify({ status }) }),
  async getNote(id: string): Promise<string> {
    const body = await request<unknown>(`/topics/${id}/note`)
    return stringValue(asRecord(body)?.content)
  },
  setNote: (id: string, content: string) => request<unknown>(`/topics/${id}/note`, { method: 'PUT', body: JSON.stringify({ content }) }),
  async getChecklist(id: string): Promise<ChecklistItem[]> {
    return items<unknown>(await request(`/topics/${id}/checklist`)).map(checklistItem)
  },
  async setChecklist(id: string, checklist: ChecklistItemInput[]): Promise<ChecklistItem[]> {
    return items<unknown>(await request(`/topics/${id}/checklist`, { method: 'PUT', body: JSON.stringify({ items: checklist }) })).map(checklistItem)
  },
  async getDocument(id: string): Promise<TopicDocument> {
    return document(await request(`/topics/${id}/document`))
  },
  async setDocument(id: string, content: string): Promise<TopicDocument> {
    return document(await request(`/topics/${id}/document`, { method: 'PUT', body: JSON.stringify({ content }) }))
  },

  async getAnalytics(): Promise<Analytics> {
    return analytics(await request('/analytics'))
  },

  async getDependencies(roadmapId: string): Promise<Dependency[]> {
    return items<unknown>(await request(`/roadmaps/${roadmapId}/dependencies`)).map(dependency)
  },
  putDependencies: (roadmapId: string, dependencies: Dependency[]) => request<unknown>(`/roadmaps/${roadmapId}/dependencies`, {
    method: 'PUT',
    body: JSON.stringify({ dependencies }),
  }),
  async getProgress(roadmapId: string): Promise<Progress> {
    return progress(await request(`/roadmaps/${roadmapId}/progress`))
  },
  async getLayout(roadmapId: string): Promise<TopicLayout[]> {
    return items<unknown>(await request(`/roadmaps/${roadmapId}/layout`)).map(layoutItem)
  },
  putLayout: (roadmapId: string, layout: TopicLayout[]) => request<unknown>(`/roadmaps/${roadmapId}/layout`, {
    method: 'PUT',
    body: JSON.stringify({ items: layout }),
  }),
  async startLearning(elementId: string): Promise<LearningSession> { const value = learningSession(await request(`/learning/sessions/start`, { method: 'POST', body: JSON.stringify({ elementId }) })); if (!value) throw new Error('Сессия не создана'); return value },
  async pauseLearning(id: string): Promise<LearningSession> { const value = learningSession(await request(`/learning/sessions/${id}/pause`, { method: 'POST' })); if (!value) throw new Error('Сессия не найдена'); return value },
  async resumeLearning(id: string): Promise<LearningSession> { const value = learningSession(await request(`/learning/sessions/${id}/resume`, { method: 'POST' })); if (!value) throw new Error('Сессия не найдена'); return value },
  async finishLearning(id: string): Promise<LearningSession> { const value = learningSession(await request(`/learning/sessions/${id}/finish`, { method: 'POST' })); if (!value) throw new Error('Сессия не найдена'); return value },
  async getActiveLearning(): Promise<LearningSession | null> { return learningSession(await request('/learning/sessions/active')) },
  getLearningSessions: () => request<LearningSession[]>('/learning/sessions'),
  async getLearningOverview(): Promise<LearningOverview> { const raw = asRecord(await request<unknown>('/learning/overview')) ?? {}; return { study_minutes: numberValue(raw.study_minutes), topic_minutes: numberValue(raw.topic_minutes), task_minutes: numberValue(raw.task_minutes), active_days: numberValue(raw.active_days), active: asRecord(raw.active) as LearningOverview['active'] } },
  async getLearningActivity(range?: { from?: string; to?: string; timezone?: string }): Promise<LearningActivity[]> { const query = range ? `?${new URLSearchParams(Object.entries(range).filter((entry): entry is [string, string] => Boolean(entry[1])))}` : ''; return items<unknown>(await request(`/learning/activity${query}`)).map((item) => { const raw = asRecord(item) ?? {}; return { date: stringValue(raw.date), study_minutes: numberValue(raw.study_minutes) } }) },
  async getLearningAchievements(): Promise<LearningAchievement[]> { return items<unknown>(await request('/learning/achievements')).map((item) => { const raw = asRecord(item) ?? {}; return { kind: raw.kind === '6h' || raw.kind === '8h' ? raw.kind : '4h', days: numberValue(raw.days) } }) },
  async getAIAnalysis() { return learningReport(await request('/learning/analysis/latest')) },
  async runAIAnalysis() { return learningReport(await request('/learning/analysis', { method: 'POST' })) },
  async uploadImage(topicId: string, file: File): Promise<{ id: string }> {
    const body = new FormData(); body.append('topic_id', topicId); body.append('file', file)
    const raw = asRecord(await request<unknown>('/uploads/images', { method: 'POST', body })) ?? {}
    return { id: stringValue(raw.id ?? raw.image_id ?? raw.key) }
  },
}
