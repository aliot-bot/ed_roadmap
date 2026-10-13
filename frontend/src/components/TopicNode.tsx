import { Handle, Position, type Node, type NodeProps } from '@xyflow/react'
import { Icon } from './Icon'
import { statusLabel, priorityLabel } from './topicPresentation'

export type TopicNodeData = {
  title: string
  kind: 'topic' | 'task'
  description: string
  completed: boolean
  status: 'not_started' | 'in_progress' | 'needs_review' | 'completed'
  priority: 'critical' | 'important' | 'optional' | 'skippable'
  taskStatus?: 'not_started' | 'in_progress' | 'completed'
  isLeaf: boolean
  childCount: number
  collapsed: boolean
  progressLabel?: string
  onToggle: (id: string, completed: boolean) => void
  onCollapse: (id: string) => void
  onSelect: (id: string) => void
}

export type TopicFlowNode = Node<TopicNodeData, 'topic'>

export function TopicNode({ id, data, selected }: NodeProps<TopicFlowNode>) {
  return (
    <article className={`topic-node ${selected ? 'selected' : ''} status-${data.status} priority-${data.priority}`} role="button" tabIndex={0} onClick={() => data.onSelect(id)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); data.onSelect(id) } }} aria-pressed={selected} aria-label={`Тема «${data.title}», статус: ${statusLabel(data.status)}, приоритет: ${priorityLabel(data.priority)}`}>
      <Handle type="target" position={Position.Left} className="topic-handle" />
      <div className="topic-node-head">
        {data.isLeaf && data.kind !== 'task' ? (
          <label className="check-control nodrag" title={data.completed ? 'Отметить как не изученное' : 'Отметить как изученное'}>
            <input type="checkbox" checked={data.completed} onChange={(event) => data.onToggle(id, event.target.checked)} aria-label={`Тема «${data.title}» изучена`} />
            <span aria-hidden="true"><Icon name="check" /></span>
          </label>
        ) : (
          <button className="collapse-button nodrag" onClick={(event) => { event.stopPropagation(); data.onCollapse(id) }} aria-expanded={!data.collapsed} aria-label={data.collapsed ? 'Показать дочерние темы' : 'Скрыть дочерние темы'}>
            <Icon name="chevron" />
          </button>
        )}
        <div>
          <h3>{data.title} <span className={`topic-kind-badge ${data.kind === 'task' ? 'is-task' : ''}`}>{data.kind === 'task' ? 'Задача' : 'Тема'}</span></h3>
          <span className="node-status">{data.kind === 'task' ? 'Задача: ' + (data.taskStatus === 'completed' ? 'Готово' : data.taskStatus === 'in_progress' ? 'В работе' : 'К выполнению') : data.isLeaf ? statusLabel(data.status) : `${data.childCount} тем · ${data.progressLabel}`}</span>
          <span className={`priority-badge priority-${data.priority}`}>{priorityLabel(data.priority)}</span>
        </div>
      </div>
      {data.description && <p>{data.description}</p>}
      <Handle type="source" position={Position.Right} className="topic-handle" />
    </article>
  )
}
