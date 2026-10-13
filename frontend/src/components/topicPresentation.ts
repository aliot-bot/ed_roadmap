export type PresentationStatus = 'not_started' | 'in_progress' | 'needs_review' | 'completed'
export type PresentationPriority = 'critical' | 'important' | 'optional' | 'skippable'
export function statusLabel(status: PresentationStatus) { return ({ not_started: 'Не начата', in_progress: 'В процессе', needs_review: 'Требует повторения', completed: 'Изучена' } as const)[status] }
export function priorityLabel(priority: PresentationPriority) { return ({ critical: 'Критично', important: 'Важно', optional: 'Дополнительно', skippable: 'Можно пропустить' } as const)[priority] }
