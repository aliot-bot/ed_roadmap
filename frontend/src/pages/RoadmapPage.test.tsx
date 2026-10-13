import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { RoadmapPage } from './RoadmapPage'

vi.mock('@xyflow/react', () => ({
  Background: () => null,
  Controls: () => null,
  MarkerType: { ArrowClosed: 'arrowclosed' },
  ReactFlow: ({ nodes, children, onInit, onNodeClick }: { nodes: Array<{ id: string; data: { title: string } }>; children: React.ReactNode; onInit?: (value: { fitView: () => void }) => void; onNodeClick?: (event: React.MouseEvent, node: { id: string }) => void }) => {
    React.useEffect(() => { onInit?.({ fitView: vi.fn() }) }, [onInit])
    return <div data-testid="flow">{nodes.map((node) => <button key={node.id} onClick={(event) => onNodeClick?.(event, node)}>{node.data.title}</button>)}{children}</div>
  },
  useNodesState: <T,>(initial: T[]) => {
    const [nodes, setNodes] = React.useState(initial)
    const onNodesChange = React.useCallback(() => undefined, [])
    return [nodes, setNodes, onNodesChange] as const
  },
}))

vi.mock('../api', () => ({
  api: {
    getRoadmap: vi.fn().mockResolvedValue({ id: 'roadmap', title: 'Frontend', description: 'Учебная карта' }),
    listTopics: vi.fn().mockResolvedValue([
      { id: 'root', roadmap_id: 'roadmap', parent_id: null, title: 'Основы', description: '', sort_order: 0, completed: false, kind: 'topic' },
      { id: 'leaf', roadmap_id: 'roadmap', parent_id: 'root', title: 'React', description: '', sort_order: 0, completed: false, kind: 'topic' },
    ]),
    getDependencies: vi.fn().mockResolvedValue([]),
    getProgress: vi.fn().mockResolvedValue({ completed: 0, total: 1, percent: 0 }),
    getLayout: vi.fn().mockResolvedValue([]),
    setCompletion: vi.fn(),
    setStatus: vi.fn().mockResolvedValue({}),
    deleteTopic: vi.fn(),
    putLayout: vi.fn(),
    getNote: vi.fn().mockResolvedValue(''),
    setNote: vi.fn(),
    updateTopic: vi.fn(),
    createTopic: vi.fn(),
    putDependencies: vi.fn(),
    startLearning: vi.fn().mockResolvedValue({ id: 'session-1', element_id: 'leaf', status: 'active', started_at: '2026-10-12T08:00:00Z', updated_at: '2026-10-12T08:00:00Z', elapsed_seconds: 0 }),
  },
}))

Object.assign(HTMLDialogElement.prototype, { showModal: vi.fn(), close: vi.fn() })

describe('RoadmapPage graph lifecycle', () => {
  it('keeps graph nodes mounted after query and flow state updates', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)

    render(
      <QueryClientProvider client={queryClient}>
        <RoadmapPage roadmapId="roadmap" onBack={vi.fn()} />
      </QueryClientProvider>,
    )

    expect(await screen.findByTestId('flow')).toBeInTheDocument()
    expect(screen.getByText('Основы')).toBeInTheDocument()
    expect(screen.getByText('React')).toBeInTheDocument()

    act(() => {
      queryClient.setQueryData(['topics', 'roadmap'], [
        { id: 'root', roadmap_id: 'roadmap', parent_id: null, title: 'Основы', description: '', sort_order: 0, completed: false },
        { id: 'leaf', roadmap_id: 'roadmap', parent_id: 'root', title: 'React', description: '', sort_order: 0, completed: false },
        { id: 'new-topic', roadmap_id: 'roadmap', parent_id: null, title: 'Новая тема', description: '', sort_order: 1, completed: false },
      ])
    })

    expect(await screen.findByText('Новая тема')).toBeInTheDocument()
    await waitFor(() => expect(consoleError).not.toHaveBeenCalledWith(expect.stringContaining('Maximum update depth exceeded')))

    consoleError.mockRestore()
  })

  it('refreshes the global timer and learning aggregates after starting a session', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    render(
      <QueryClientProvider client={queryClient}>
        <RoadmapPage roadmapId="roadmap" onBack={vi.fn()} />
      </QueryClientProvider>,
    )

    fireEvent.click(await screen.findByRole('button', { name: 'React' }))
    fireEvent.click(await screen.findByRole('button', { name: /Начать занятие/ }))

    await waitFor(() => expect(api.startLearning).toHaveBeenCalledWith('leaf'))
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ['learning-active'] }))
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['learning-overview'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['learning-activity'] })
  })

  it('saves a topic status and exposes the priority field in the editor', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(<QueryClientProvider client={queryClient}><RoadmapPage roadmapId="roadmap" onBack={vi.fn()} /></QueryClientProvider>)
    fireEvent.click(await screen.findByRole('button', { name: 'React' }))
    fireEvent.change(screen.getByLabelText('Статус освоения'), { target: { value: 'needs_review' } })
    await waitFor(() => expect(api.setStatus).toHaveBeenCalledWith('leaf', 'needs_review'))
    fireEvent.click(screen.getByRole('button', { name: /Изменить/ }))
    const priorityFields = await screen.findAllByLabelText('Приоритет')
    expect(priorityFields.at(-1)).toBeInTheDocument()
    fireEvent.change(priorityFields.at(-1)!, { target: { value: 'critical' } })
    fireEvent.submit(priorityFields.at(-1)!.closest('form')!)
    await waitFor(() => expect(api.updateTopic).toHaveBeenCalledWith('leaf', expect.objectContaining({ priority: 'critical' })))
  })

  it('auto-lays out visible graph nodes and persists every topic position', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(<QueryClientProvider client={queryClient}><RoadmapPage roadmapId="roadmap" onBack={vi.fn()} /></QueryClientProvider>)

    fireEvent.click(await screen.findByRole('button', { name: /Авторазложить/ }))

    await waitFor(() => expect(api.putLayout).toHaveBeenCalledWith('roadmap', expect.arrayContaining([
      expect.objectContaining({ topic_id: 'root' }),
      expect.objectContaining({ topic_id: 'leaf' }),
    ])))
  })

  it('uses only the graph canvas as the fullscreen target and synchronizes native exits', async () => {
    const previousRequest = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'requestFullscreen')
    const previousEnabled = Object.getOwnPropertyDescriptor(document, 'fullscreenEnabled')
    const previousElement = Object.getOwnPropertyDescriptor(document, 'fullscreenElement')
    const requestFullscreen = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(HTMLElement.prototype, 'requestFullscreen', { configurable: true, value: requestFullscreen })
    Object.defineProperty(document, 'fullscreenEnabled', { configurable: true, value: true })
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, writable: true, value: null })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })

    render(<QueryClientProvider client={queryClient}><RoadmapPage roadmapId="roadmap" onBack={vi.fn()} /></QueryClientProvider>)
    const canvas = await screen.findByLabelText('Граф карты знаний')
    const fullscreenButton = screen.getByRole('button', { name: 'Открыть граф на весь экран' })
    fireEvent.click(fullscreenButton)
    await waitFor(() => expect(requestFullscreen).toHaveBeenCalledWith())
    expect(requestFullscreen.mock.instances[0]).toBe(canvas)

    Object.defineProperty(document, 'fullscreenElement', { configurable: true, writable: true, value: canvas })
    fireEvent(document, new Event('fullscreenchange'))
    expect((await screen.findAllByRole('button', { name: /Выйти из полноэкранного режима/ })).length).toBe(2)

    Object.defineProperty(document, 'fullscreenElement', { configurable: true, writable: true, value: null })
    fireEvent(document, new Event('fullscreenchange'))
    await waitFor(() => expect(screen.queryAllByRole('button', { name: /Выйти из полноэкранного режима/ })).toHaveLength(0))

    if (previousRequest) Object.defineProperty(HTMLElement.prototype, 'requestFullscreen', previousRequest)
    else delete (HTMLElement.prototype as { requestFullscreen?: () => Promise<void> }).requestFullscreen
    if (previousEnabled) Object.defineProperty(document, 'fullscreenEnabled', previousEnabled)
    else delete (document as { fullscreenEnabled?: boolean }).fullscreenEnabled
    if (previousElement) Object.defineProperty(document, 'fullscreenElement', previousElement)
    else delete (document as { fullscreenElement?: Element | null }).fullscreenElement
  })
})
