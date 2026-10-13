import type { Topic } from '../types'
import { childTopics } from '../graph'
import { Icon } from './Icon'

type TopicTreeProps = {
  topics: Topic[]
  selectedId?: string
  collapsed: Set<string>
  onSelect: (topic: Topic) => void
  onToggle: (topic: Topic, completed: boolean) => void
  onCollapse: (id: string) => void
  onStartLearning?: (topic: Topic) => void
}

export function TopicTree(props: TopicTreeProps) {
  const roots = props.topics.filter((topic) => topic.parent_id === null || !props.topics.some((candidate) => candidate.id === topic.parent_id))
  if (roots.length === 0) return <div className="canvas-empty"><Icon name="map" /><h2>Добавьте первую тему</h2><p>Она станет отправной точкой вашей карты.</p></div>
  return <div className="topic-tree" role="tree" aria-label="Темы карты">{sort(roots).map((topic) => <TreeItem key={topic.id} topic={topic} level={1} {...props} />)}</div>
}

function TreeItem({ topic, level, ...props }: TopicTreeProps & { topic: Topic; level: number }) {
  const children = sort(childTopics(props.topics, topic.id))
  const isLeaf = children.length === 0
  const isCollapsed = props.collapsed.has(topic.id)
  return (
    <div role="treeitem" aria-level={level} aria-expanded={isLeaf ? undefined : !isCollapsed} aria-selected={topic.id === props.selectedId}>
      <div className={`tree-row ${topic.id === props.selectedId ? 'selected' : ''}`} data-topic-id={topic.id} style={{ '--tree-level': level } as React.CSSProperties}>
        {isLeaf && topic.kind !== 'task' ? (
          <label className="check-control">
            <input type="checkbox" checked={topic.completed} onChange={(event) => props.onToggle(topic, event.target.checked)} aria-label={`Тема «${topic.title}» изучена`} />
            <span aria-hidden="true"><Icon name="check" /></span>
          </label>
        ) : (
          <button className="collapse-button" onClick={() => props.onCollapse(topic.id)} aria-label={isCollapsed ? 'Показать дочерние темы' : 'Скрыть дочерние темы'}><Icon name="chevron" /></button>
        )}
        <button className="tree-topic" onClick={() => props.onSelect(topic)}>
          <strong>{topic.title}</strong> <span className={`topic-kind-badge ${topic.kind === 'task' ? 'is-task' : ''}`}>{topic.kind === 'task' ? 'Задача' : 'Тема'}</span>
          <small><span className={`status-dot status-${topic.kind === 'task' ? (topic.task_status ?? 'not_started') : (topic.status ?? (topic.completed ? 'completed' : 'not_started'))}`} aria-hidden="true" />{topic.kind === 'task' ? (topic.task_status === 'completed' ? 'Готово' : topic.task_status === 'in_progress' ? 'В работе' : 'К выполнению') : isLeaf ? statusLabel(topic.status ?? (topic.completed ? 'completed' : 'not_started')) : `${children.length} вложенных тем`} · {priorityLabel(topic.priority ?? 'important')}</small>
        </button>
        {topic.id === props.selectedId && props.onStartLearning && <button className="tree-start-button" onClick={() => props.onStartLearning?.(topic)} aria-label={`Начать занятие по теме «${topic.title}»`}><Icon name="play" /><span>Начать</span></button>}
      </div>
      {!isLeaf && !isCollapsed && <div role="group">{children.map((child) => <TreeItem key={child.id} topic={child} level={level + 1} {...props} />)}</div>}
    </div>
  )
}

function statusLabel(status: NonNullable<Topic['status']>) { return ({ not_started: 'Не начата', in_progress: 'В процессе', needs_review: 'Требует повторения', completed: 'Изучена' } as const)[status] }
function priorityLabel(priority: NonNullable<Topic['priority']>) { return ({ critical: 'Критично', important: 'Важно', optional: 'Дополнительно', skippable: 'Можно пропустить' } as const)[priority] }

function sort(topics: Topic[]) {
  return [...topics].sort((left, right) => left.sort_order - right.sort_order || left.title.localeCompare(right.title, 'ru'))
}
