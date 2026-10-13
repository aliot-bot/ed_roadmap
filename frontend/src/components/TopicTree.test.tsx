import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TopicTree } from './TopicTree'
import { collapsedAtLevel } from '../graph'
import type { Topic } from '../types'

const topics: Topic[] = [
  { id: 'group', roadmap_id: 'roadmap', parent_id: null, title: 'Раздел', description: '', sort_order: 0, completed: false },
  { id: 'leaf', roadmap_id: 'roadmap', parent_id: 'group', title: 'Конечная тема', description: '', sort_order: 0, completed: false },
]

describe('TopicTree', () => {
  it('selects a topic and changes completion only for a leaf', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    const onToggle = vi.fn()
    render(<TopicTree topics={topics} collapsed={new Set()} onSelect={onSelect} onToggle={onToggle} onCollapse={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: /Конечная тема/ }))
    await user.click(screen.getByRole('checkbox', { name: 'Тема «Конечная тема» изучена' }))

    expect(onSelect).toHaveBeenCalledWith(topics[1])
    expect(onToggle).toHaveBeenCalledWith(topics[1], true)
  })

  it('offers collapse controls for groups and hides children after state update', async () => {
    const user = userEvent.setup()
    const onCollapse = vi.fn()
    const { rerender } = render(<TopicTree topics={topics} collapsed={new Set()} onSelect={vi.fn()} onToggle={vi.fn()} onCollapse={onCollapse} />)

    await user.click(screen.getByRole('button', { name: 'Скрыть дочерние темы' }))
    expect(onCollapse).toHaveBeenCalledWith('group')

    rerender(<TopicTree topics={topics} collapsed={new Set(['group'])} onSelect={vi.fn()} onToggle={vi.fn()} onCollapse={onCollapse} />)
    expect(screen.queryByText('Конечная тема')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Показать дочерние темы' })).toBeInTheDocument()
  })

  it('renders and collapses a long chain without losing the visible ancestor path', async () => {
    const user = userEvent.setup()
    const chain = Array.from({ length: 320 }, (_, index): Topic => ({
      id: `topic-${index}`,
      roadmap_id: 'roadmap',
      parent_id: index === 0 ? null : `topic-${index - 1}`,
      title: `Тема ${index}`,
      description: '',
      sort_order: 0,
      completed: false,
    }))
    const onCollapse = vi.fn()
    const { container, rerender } = render(<TopicTree topics={chain} collapsed={new Set()} onSelect={vi.fn()} onToggle={vi.fn()} onCollapse={onCollapse} />)

    expect(container.querySelectorAll('[role="treeitem"]')).toHaveLength(chain.length)
    await user.click(container.querySelectorAll('button[aria-label="Скрыть дочерние темы"]')[120]!)
    expect(onCollapse).toHaveBeenCalledWith('topic-120')

    rerender(<TopicTree topics={chain} collapsed={new Set(['topic-120'])} onSelect={vi.fn()} onToggle={vi.fn()} onCollapse={onCollapse} />)
    expect(container.querySelectorAll('[role="treeitem"]')).toHaveLength(121)
    expect([...container.querySelectorAll('strong')].some((node) => node.textContent === 'Тема 120')).toBe(true)
    expect([...container.querySelectorAll('strong')].some((node) => node.textContent === 'Тема 121')).toBe(false)
  })

  it('collapses every branch at a requested structure level', () => {
    const nested = [
      topics[0]!,
      { ...topics[1]!, id: 'middle', parent_id: 'group', title: 'Подраздел' },
      { ...topics[1]!, id: 'deep-leaf', parent_id: 'middle', title: 'Глубокая тема' },
    ]

    expect([...collapsedAtLevel(nested, 2)]).toEqual(['middle'])
  })

  it('places the learning action beside the selected topic', async () => {
    const user = userEvent.setup()
    const onStartLearning = vi.fn()
    render(<TopicTree topics={topics} selectedId="leaf" collapsed={new Set()} onSelect={vi.fn()} onToggle={vi.fn()} onCollapse={vi.fn()} onStartLearning={onStartLearning} />)

    await user.click(screen.getByRole('button', { name: 'Начать занятие по теме «Конечная тема»' }))
    expect(onStartLearning).toHaveBeenCalledWith(topics[1])
  })
})
