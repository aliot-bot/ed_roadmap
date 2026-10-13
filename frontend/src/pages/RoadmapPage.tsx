import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Background,
  Controls,
  MarkerType,
  ReactFlow,
  type Edge,
  type ReactFlowInstance,
  useNodesState,
} from '@xyflow/react'
import { api } from '../api'
import { autoLayoutTopics, collapsedAtLevel, descendantsOf, hiddenTopicIds, topicDepth as structureDepth, topicProgress, wouldCreateDependencyCycle } from '../graph'
import type { Dependency, Topic, TopicLayout, TopicPriority } from '../types'
import { Dialog, ErrorMessage } from '../components/Dialog'
import { Icon } from '../components/Icon'
import { ProgressBar } from '../components/ProgressBar'
import { TopicNode, type TopicFlowNode, type TopicNodeData } from '../components/TopicNode'
import { TopicTree } from '../components/TopicTree'
import { TopicChecklist } from '../components/TopicChecklist'

const TopicDocumentDialog = lazy(() => import('../components/TopicDocument').then((module) => ({ default: module.TopicDocumentDialog })))

const nodeTypes = { topic: TopicNode }
type MapView = 'graph' | 'tree'

export function RoadmapPage({ roadmapId, onBack }: { roadmapId: string; onBack: () => void }) {
  const queryClient = useQueryClient()
  const roadmap = useQuery({ queryKey: ['roadmap', roadmapId], queryFn: () => api.getRoadmap(roadmapId) })
  const topics = useQuery({ queryKey: ['topics', roadmapId], queryFn: () => api.listTopics(roadmapId) })
  const dependencies = useQuery({ queryKey: ['dependencies', roadmapId], queryFn: () => api.getDependencies(roadmapId) })
  const progress = useQuery({ queryKey: ['progress', roadmapId], queryFn: () => api.getProgress(roadmapId) })
  const layout = useQuery({ queryKey: ['layout', roadmapId], queryFn: () => api.getLayout(roadmapId) })
  const [view, setView] = useState<MapView>(() => window.matchMedia?.('(max-width: 720px)').matches ? 'tree' : 'graph')
  const [kindFilter, setKindFilter] = useState<'all' | 'topic' | 'task'>('all')
  const [priorityFilter, setPriorityFilter] = useState<'all' | TopicPriority>('all')
  const [sortPriority, setSortPriority] = useState(false)
  const [selectedId, setSelectedId] = useState<string>()
  const [topicPanelOffset, setTopicPanelOffset] = useState(0)
  const [collapsedChanges, setCollapsedChanges] = useState<Map<string, boolean>>(new Map())
  const [topicForm, setTopicForm] = useState<{ topic?: Topic; parentId?: string | null } | null>(null)
  const [dependenciesOpen, setDependenciesOpen] = useState(false)
  const [deleting, setDeleting] = useState<Topic | null>(null)
  const [flow, setFlow] = useState<ReactFlowInstance<TopicFlowNode, Edge> | null>(null)
  const [nodes, setNodes, onNodesChange] = useNodesState<TopicFlowNode>([])
  const nodesRef = useRef<TopicFlowNode[]>([])
  const mapCanvasRef = useRef<HTMLElement>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [fullscreenError, setFullscreenError] = useState<string>()
  const [fullscreenSupported] = useState(() => typeof document !== 'undefined' && document.fullscreenEnabled !== false && typeof HTMLElement.prototype.requestFullscreen === 'function')

  const invalidateMap = useCallback(() => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['roadmap', roadmapId] }),
    queryClient.invalidateQueries({ queryKey: ['topics', roadmapId] }),
    queryClient.invalidateQueries({ queryKey: ['progress', roadmapId] }),
    queryClient.invalidateQueries({ queryKey: ['roadmaps'] }),
    queryClient.invalidateQueries({ queryKey: ['analytics'] }),
  ]), [queryClient, roadmapId])

  const completion = useMutation({
    mutationFn: ({ id, completed }: { id: string; completed: boolean }) => api.setCompletion(id, completed),
    onSuccess: () => { void invalidateMap() },
  })
  const startLearningFromTree = useMutation({
    mutationFn: (id: string) => api.startLearning(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['learning-active'] })
      void queryClient.invalidateQueries({ queryKey: ['learning-overview'] })
      void queryClient.invalidateQueries({ queryKey: ['learning-activity'] })
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => api.deleteTopic(id),
    onSuccess: () => { setDeleting(null); setSelectedId(undefined); void invalidateMap(); void queryClient.invalidateQueries({ queryKey: ['dependencies', roadmapId] }) },
  })
  const saveLayout = useMutation({
    mutationFn: (items: TopicLayout[]) => api.putLayout(roadmapId, items),
    onSuccess: (_, items) => queryClient.setQueryData(['layout', roadmapId], items),
  })
  const mutateCompletion = completion.mutate
  const mutateLayout = saveLayout.mutate

  useEffect(() => {
    nodesRef.current = nodes
  }, [nodes])

  const collapsed = useMemo(() => {
    const next = new Set(layout.data?.filter((item) => item.collapsed).map((item) => item.topic_id) ?? [])
    for (const [id, value] of collapsedChanges) {
      if (value) next.add(id)
      else next.delete(id)
    }
    return next
  }, [collapsedChanges, layout.data])

  const persistLayout = useCallback((nextCollapsed: Set<string>, positionedNodes = nodesRef.current) => {
    if (!topics.data) return
    const positions = new Map(layout.data?.map((item) => [item.topic_id, { x: item.x, y: item.y }]) ?? [])
    positionedNodes.forEach((node) => positions.set(node.id, node.position))
    mutateLayout(topics.data.map((topic, index) => {
      const position = positions.get(topic.id) ?? defaultPosition(topic, topics.data, index)
      return { topic_id: topic.id, x: Math.round(position.x), y: Math.round(position.y), collapsed: nextCollapsed.has(topic.id) }
    }))
  }, [layout.data, mutateLayout, topics.data])

  const toggleCompletion = useCallback((id: string, completed: boolean) => {
    mutateCompletion({ id, completed })
  }, [mutateCompletion])

  const toggleCollapse = useCallback((id: string) => {
    const next = new Set(collapsed)
    const value = !next.has(id)
    if (value) next.add(id)
    else next.delete(id)
    setCollapsedChanges((current) => new Map(current).set(id, value))
    persistLayout(next)
  }, [collapsed, persistLayout])

  const setVisibleLevels = useCallback((level?: number) => {
    if (!topics.data) return
    const next = level === undefined ? new Set<string>() : collapsedAtLevel(topics.data, level)
    setCollapsedChanges(new Map(topics.data.map((topic) => [topic.id, next.has(topic.id)])))
    if (selectedId && hiddenTopicIds(topics.data, next).has(selectedId)) setSelectedId(undefined)
    persistLayout(next)
  }, [persistLayout, selectedId, topics.data])

  const treeLevels = useMemo(() => {
    if (!topics.data) return []
    const maxDepth = Math.max(...topics.data.map((topic) => structureDepth(topic, topics.data)))
    return Array.from({ length: Math.min(maxDepth, 4) }, (_, index) => index + 1)
  }, [topics.data])

  useLayoutEffect(() => {
    if (view !== 'tree' || !selectedId) {
      const frame = window.requestAnimationFrame(() => setTopicPanelOffset(0))
      return () => window.cancelAnimationFrame(frame)
    }
    const canvas = mapCanvasRef.current
    if (!canvas) return
    const syncPanelPosition = () => {
      const row = [...canvas.querySelectorAll<HTMLElement>('[data-topic-id]')].find((element) => element.dataset.topicId === selectedId)
      if (!row) return
      const canvasTop = canvas.getBoundingClientRect().top
      const rowTop = row.getBoundingClientRect().top
      setTopicPanelOffset(Math.max(0, Math.round(rowTop - canvasTop + canvas.scrollTop - 10)))
    }
    const frame = window.requestAnimationFrame(syncPanelPosition)
    canvas.addEventListener('scroll', syncPanelPosition, { passive: true })
    window.addEventListener('resize', syncPanelPosition)
    return () => {
      window.cancelAnimationFrame(frame)
      canvas.removeEventListener('scroll', syncPanelPosition)
      window.removeEventListener('resize', syncPanelPosition)
    }
  }, [selectedId, view])

  const flowNodes = useMemo<TopicFlowNode[]>(() => {
    if (!topics.data) return []
    const positions = new Map(layout.data?.map((item) => [item.topic_id, item]) ?? [])
    const hidden = hiddenTopicIds(topics.data, collapsed)
    return topics.data.filter((topic) => !hidden.has(topic.id) && (kindFilter === 'all' || topic.kind === kindFilter) && (priorityFilter === 'all' || (topic.priority ?? 'important') === priorityFilter)).sort((a, b) => sortPriority ? priorityRank(a.priority) - priorityRank(b.priority) : 0).map((topic, index) => {
      const childCount = topics.data.filter((item) => item.parent_id === topic.id).length
      const itemProgress = topicProgress(topic.id, topics.data)
      const saved = positions.get(topic.id)
      const data: TopicNodeData = {
        title: topic.title,
        kind: topic.kind ?? 'topic',
        description: topic.description,
        completed: topic.completed,
        status: topic.kind === 'task' ? (topic.task_status ?? 'not_started') : (topic.status ?? (topic.completed ? 'completed' : 'not_started')),
        priority: topic.priority ?? 'important',
        taskStatus: topic.task_status ?? 'not_started',
        isLeaf: childCount === 0,
        childCount,
        collapsed: collapsed.has(topic.id),
        progressLabel: itemProgress.total ? `${itemProgress.percent}% изучено` : 'нет конечных тем',
        onToggle: toggleCompletion,
        onCollapse: toggleCollapse,
        onSelect: setSelectedId,
      }
      return { id: topic.id, type: 'topic', position: saved ? { x: saved.x, y: saved.y } : defaultPosition(topic, topics.data, index), data }
    })
  }, [collapsed, kindFilter, layout.data, priorityFilter, sortPriority, toggleCollapse, toggleCompletion, topics.data])

  const scheduleFitView = useCallback((padding: number, duration = 180, delay = 0) => {
    if (!flow) return
    // React Flow needs one rendered frame to measure moved nodes and a second
    // frame to apply the new viewport bounds. Fitting sooner can use stale
    // coordinates, which leaves a correctly laid-out graph outside the view.
    const fit = () => window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => flow.fitView({ padding, duration }))
    })
    if (delay) window.setTimeout(fit, delay)
    else fit()
  }, [flow])

  useEffect(() => setNodes(flowNodes), [flowNodes, setNodes])
  useEffect(() => {
    if (!flow || nodes.length === 0) return
    const task = window.setTimeout(() => scheduleFitView(0.2), 0)
    return () => window.clearTimeout(task)
  }, [flow, flowNodes, nodes.length, scheduleFitView])

  useEffect(() => {
    const syncFullscreen = () => {
      const active = document.fullscreenElement === mapCanvasRef.current
      setIsFullscreen(active)
      if (active) setFullscreenError(undefined)
      // Fullscreen dimensions are committed after the event, so wait for the
      // new canvas bounds before centering. This prevents an off-centre graph
      // on the first fullscreen frame.
      if (active) scheduleFitView(0.14, 0, 120)
    }
    document.addEventListener('fullscreenchange', syncFullscreen)
    return () => document.removeEventListener('fullscreenchange', syncFullscreen)
  }, [scheduleFitView])

  const toggleFullscreen = useCallback(async () => {
    const canvas = mapCanvasRef.current
    if (!canvas || !fullscreenSupported) return
    try {
      if (document.fullscreenElement === canvas) await document.exitFullscreen()
      else await canvas.requestFullscreen()
    } catch {
      setFullscreenError('Полноэкранный режим не удалось включить. Проверьте настройки браузера.')
    }
  }, [fullscreenSupported])

  const applyAutoLayout = useCallback(() => {
    if (!topics.data || nodesRef.current.length === 0) return
    const visibleIds = new Set(nodesRef.current.map((node) => node.id))
    const visibleTopics = topics.data.filter((topic) => visibleIds.has(topic.id))
    const sizes = new Map(nodesRef.current.map((node) => [node.id, {
      width: node.measured?.width ?? node.width ?? 272,
      height: node.measured?.height ?? node.height ?? 200,
    }]))
    const positions = autoLayoutTopics(visibleTopics, dependencies.data ?? [], sizes)
    const nextNodes = nodesRef.current.map((node) => ({ ...node, position: positions.get(node.id) ?? node.position }))
    setNodes(nextNodes)
    persistLayout(collapsed, nextNodes)
    scheduleFitView(0.18, 0)
  }, [collapsed, dependencies.data, persistLayout, scheduleFitView, setNodes, topics.data])

  const edges = useMemo<Edge[]>(() => {
    if (!topics.data) return []
    const visible = new Set(nodes.map((node) => node.id))
    const hierarchy = topics.data.filter((topic) => topic.parent_id && visible.has(topic.id) && visible.has(topic.parent_id)).map((topic) => ({
      id: `parent-${topic.parent_id}-${topic.id}`,
      source: topic.parent_id as string,
      target: topic.id,
      type: 'smoothstep',
      animated: false,
      style: { stroke: 'var(--graph-edge)', strokeDasharray: '5 5' },
    }))
    const prereqs = (dependencies.data ?? []).filter((edge) => visible.has(edge.predecessor_topic_id) && visible.has(edge.successor_topic_id)).map((edge) => ({
      id: `dependency-${edge.predecessor_topic_id}-${edge.successor_topic_id}`,
      source: edge.predecessor_topic_id,
      target: edge.successor_topic_id,
      type: 'smoothstep',
      markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--graph-edge-strong)' },
      style: { stroke: 'var(--graph-edge-strong)', strokeWidth: 2 },
    }))
    return [...hierarchy, ...prereqs]
  }, [dependencies.data, nodes, topics.data])

  const selected = topics.data?.find((topic) => topic.id === selectedId)
  const isPending = roadmap.isPending || topics.isPending || dependencies.isPending || progress.isPending || layout.isPending
  const error = roadmap.error ?? topics.error ?? progress.error ?? layout.error

  if (isPending) return <main className="page roadmap-page"><section className="map-status" role="status">Загружаем карту…</section></main>
  if (error || !roadmap.data || !topics.data || !progress.data) {
    return <main className="page roadmap-page"><section className="map-status"><h1>Карта не загрузилась</h1><ErrorMessage error={error ?? new Error('Карта не найдена')} /><button className="button secondary" onClick={() => { void roadmap.refetch(); void topics.refetch(); void dependencies.refetch(); void progress.refetch(); void layout.refetch() }}>Повторить</button></section></main>
  }

  return (
    <div className="page roadmap-page" data-testid="roadmap-page">
      <div className="map-header">
        <button className="button back-button" onClick={onBack}><Icon name="arrow-left" /> Все карты</button>
        <div className="map-title">
          <h1>{roadmap.data.title}</h1>
          {roadmap.data.description && <p className="subtitle">{roadmap.data.description}</p>}
        </div>
        <div className="map-progress"><ProgressBar progress={progress.data} /><div className="status-summary" aria-label="Распределение статусов конечных тем"><span className="status-neutral"><i />Не начата <strong>{progress.data.status_counts?.not_started ?? 0}</strong></span><span className="status-active"><i />В процессе <strong>{progress.data.status_counts?.in_progress ?? 0}</strong></span><span className="status-review"><i />Повторить <strong>{progress.data.status_counts?.needs_review ?? 0}</strong></span><span className="status-done"><i />Изучена <strong>{progress.data.status_counts?.completed ?? progress.data.completed}</strong></span></div></div>
      </div>

      <div className="map-toolbar" role="toolbar" aria-label="Управление картой">
        <div className="map-toolbar-context">
          <div className="view-switch" role="group" aria-label="Представление карты">
            <button className={view === 'graph' ? 'active' : ''} aria-pressed={view === 'graph'} onClick={() => setView('graph')}><Icon name="graph" /> Граф</button>
            <button className={view === 'tree' ? 'active' : ''} aria-pressed={view === 'tree'} onClick={() => { if (isFullscreen) void document.exitFullscreen(); setView('tree') }}><Icon name="list" /> Список</button>
          </div>
          <div className="toolbar-filters">
            <label className="kind-filter">Тип<select value={kindFilter} onChange={(event) => setKindFilter(event.target.value as typeof kindFilter)}><option value="all">Все</option><option value="topic">Темы</option><option value="task">Задачи</option></select></label>
            <label className="kind-filter">Приоритет<select value={priorityFilter} onChange={(event) => setPriorityFilter(event.target.value as typeof priorityFilter)}><option value="all">Все</option><option value="critical">Критично</option><option value="important">Важно</option><option value="optional">Дополнительно</option><option value="skippable">Можно пропустить</option></select></label>
            <button className={`button tertiary ${sortPriority ? 'active' : ''}`} aria-pressed={sortPriority} onClick={() => setSortPriority((value) => !value)}>Сначала важные</button>
          </div>
        </div>
        <div className={`toolbar-actions ${view}-actions`}>
          {view === 'graph' && <div className="graph-navigation" aria-label="Навигация по графу">
            <button className="button secondary" onClick={() => scheduleFitView(0.2, 180)}><Icon name="fit" /> Весь граф</button>
            <button className="button secondary" onClick={applyAutoLayout}><Icon name="layout" /> Авторазложить</button>
            <button className="button secondary" onClick={() => void toggleFullscreen()} disabled={!fullscreenSupported} aria-pressed={isFullscreen} aria-label={fullscreenSupported ? (isFullscreen ? 'Выйти из полноэкранного режима графа' : 'Открыть граф на весь экран') : 'Полноэкранный режим недоступен в этом браузере'} title={fullscreenSupported ? undefined : 'Полноэкранный режим недоступен в этом браузере'}><Icon name={isFullscreen ? 'exit-fullscreen' : 'fullscreen'} /> {isFullscreen ? 'Выйти из экрана' : 'На весь экран'}</button>
          </div>}
          {view === 'tree' && <div className="tree-navigation" aria-label="Отображение уровней структуры"><span>Показать до уровня</span>{treeLevels.map((level) => <button key={level} className="button tertiary tree-level-button" onClick={() => setVisibleLevels(level)}>{level === 4 && treeLevels.length === 4 ? '4+' : level}</button>)}<button className="button secondary" onClick={() => setVisibleLevels()}>Развернуть всё</button></div>}
          <div className="graph-management">
            <button className="button secondary" onClick={() => setDependenciesOpen(true)}><Icon name="link" /> Связи</button>
            <button className="button primary" onClick={() => setTopicForm({ parentId: null })}><Icon name="plus" /> Добавить тему</button>
          </div>
        </div>
      </div>

      <div className="map-content">
        <section ref={mapCanvasRef} className={`map-canvas ${view === 'tree' ? 'tree-view' : ''}`} aria-label={view === 'graph' ? 'Граф карты знаний' : 'Структура карты'}>
          {view === 'graph' ? (
            nodes.length ? <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onNodeClick={(_, node) => setSelectedId(node.id)}
              onNodeDragStop={(_, __, currentNodes) => persistLayout(collapsed, currentNodes)}
              onInit={setFlow}
              fitView
              minZoom={0.03}
              maxZoom={2}
              nodesConnectable={false}
              proOptions={{ hideAttribution: true }}
            >
              <Background gap={22} size={1} color="var(--graph-grid)" />
              <Controls showInteractive={false} />
              <div className="graph-legend" aria-label="Легенда связей"><span><i className="hierarchy-line" aria-hidden="true" /> Структура</span><span><i className="dependency-line" aria-hidden="true" /> Зависимость</span></div>
              {isFullscreen && <button className="graph-fullscreen-exit button secondary" onClick={() => void toggleFullscreen()}><Icon name="exit-fullscreen" /> Выйти из полноэкранного режима</button>}
            </ReactFlow> : <EmptyTopics onCreate={() => setTopicForm({ parentId: null })} />
          ) : (
            <TopicTree topics={topics.data.filter((topic) => (kindFilter === 'all' || topic.kind === kindFilter) && (priorityFilter === 'all' || (topic.priority ?? 'important') === priorityFilter))} selectedId={selectedId} collapsed={collapsed} onCollapse={toggleCollapse} onSelect={(topic) => setSelectedId(topic.id)} onToggle={(topic, completed) => completion.mutate({ id: topic.id, completed })} onStartLearning={(topic) => startLearningFromTree.mutate(topic.id)} />
          )}
        </section>
        <aside className={`topic-panel ${view === 'tree' && selected ? 'tree-topic-panel' : ''}`} aria-label="Карточка темы" style={view === 'tree' && selected ? { '--topic-panel-offset': `${topicPanelOffset}px` } as React.CSSProperties : undefined}>
          {selected ? <TopicDetails key={selected.id} topic={selected} topics={topics.data} onEdit={() => setTopicForm({ topic: selected })} onAddChild={() => setTopicForm({ parentId: selected.id })} onDelete={() => setDeleting(selected)} onMove={(direction) => moveTopic(direction, selected, topics.data, api, invalidateMap)} /> : <div className="topic-panel-empty"><span className="panel-empty-icon"><Icon name="note" /></span><h2>Выберите тему</h2><p>Здесь появятся описание, заметки и действия с выбранной темой.</p></div>}
        </aside>
      </div>

      {dependencies.isError && <p className="inline-error" role="alert">Связи временно недоступны. Структура тем продолжает работать.</p>}
      {completion.isError && <p className="inline-error" role="alert">Не удалось сохранить отметку. Попробуйте ещё раз.</p>}
      {startLearningFromTree.isError && <p className="inline-error" role="alert">Не удалось начать занятие. Попробуйте ещё раз.</p>}
      {saveLayout.isError && <p className="inline-error" role="alert">Положение узлов не сохранилось. Данные темы не потеряны.</p>}
      {fullscreenError && <p className="inline-error" role="status">{fullscreenError}</p>}
      {topicForm && <TopicForm roadmapId={roadmapId} topics={topics.data} initial={topicForm} onClose={() => setTopicForm(null)} onSaved={(topic) => { setTopicForm(null); setSelectedId(topic.id); void invalidateMap() }} />}
      {dependenciesOpen && <DependenciesDialog roadmapId={roadmapId} topics={topics.data} dependencies={dependencies.data ?? []} onClose={() => setDependenciesOpen(false)} />}
      {deleting && <Dialog title="Удалить тему?" onClose={() => setDeleting(null)}><div className="dialog-body"><p>«{deleting.title}» будет удалена вместе с дочерними темами, связями, заметками, конспектами и отметками. Это действие нельзя отменить.</p><ErrorMessage error={remove.error} /><div className="dialog-actions"><button className="button secondary" onClick={() => setDeleting(null)}>Отмена</button><button className="button danger" disabled={remove.isPending} onClick={() => remove.mutate(deleting.id)}>Удалить тему</button></div></div></Dialog>}
    </div>
  )
}

function TopicDetails({ topic, topics, onEdit, onAddChild, onDelete, onMove }: { topic: Topic; topics: Topic[]; onEdit: () => void; onAddChild: () => void; onDelete: () => void; onMove: (direction: -1 | 1) => void }) {
  const children = topics.filter((item) => item.parent_id === topic.id)
  const progress = topicProgress(topic.id, topics)
  const siblings = topics.filter((item) => item.parent_id === topic.parent_id).sort((left, right) => left.sort_order - right.sort_order)
  const siblingIndex = siblings.findIndex((item) => item.id === topic.id)
  const queryClient = useQueryClient()
  const noteQuery = useQuery({ queryKey: ['note', topic.id], queryFn: () => api.getNote(topic.id) })
  const [noteDraft, setNoteDraft] = useState<string | null>(null)
  const [documentOpen, setDocumentOpen] = useState(false)
  const noteValue = noteDraft ?? noteQuery.data ?? ''
  const note = useMutation({
    mutationFn: (content: string) => api.setNote(topic.id, content),
    onSuccess: (_, content) => {
      queryClient.setQueryData(['note', topic.id], content)
      setNoteDraft(null)
      void queryClient.invalidateQueries({ queryKey: ['note', topic.id] })
    },
  })
  const learning = useMutation({
    mutationFn: () => api.startLearning(topic.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['learning-active'] })
      void queryClient.invalidateQueries({ queryKey: ['learning-overview'] })
      void queryClient.invalidateQueries({ queryKey: ['learning-activity'] })
    },
  })
  const currentStatus = topic.status ?? (topic.completed ? 'completed' : 'not_started')
  const statusMutation = useMutation({ mutationFn: (status: 'not_started' | 'in_progress' | 'needs_review' | 'completed') => api.setStatus(topic.id, status), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['topics', topic.roadmap_id] }); void queryClient.invalidateQueries({ queryKey: ['progress', topic.roadmap_id] }) } })
  return <div className="topic-details"><div className="panel-heading"><p className="eyebrow">{topic.kind === 'task' ? 'Задача' : 'Тема'}</p><h2>{topic.title}</h2><span className={`topic-kind-badge ${topic.kind === 'task' ? 'is-task' : ''}`}>{topic.kind === 'task' ? topic.task_status === 'completed' ? 'Готово' : topic.task_status === 'in_progress' ? 'В работе' : 'К выполнению' : 'Тема'}</span></div>{topic.kind === 'topic' && <label className="status-control">Статус освоения<select value={currentStatus} disabled={statusMutation.isPending} onChange={(event) => statusMutation.mutate(event.target.value as typeof currentStatus)}><option value="not_started">Не начата</option><option value="in_progress">В процессе</option><option value="needs_review">Требует повторения</option><option value="completed">Изучена</option></select></label>}<ErrorMessage error={statusMutation.error} />{topic.description ? <p className="topic-description">{topic.description}</p> : <p className="muted">Описание пока не добавлено.</p>}{children.length ? <ProgressBar progress={progress} /> : <p className="leaf-hint"><Icon name="check" /> Конечная тема</p>}<div className="topic-actions"><button className="button primary" onClick={() => learning.mutate()} disabled={learning.isPending}><Icon name="play" /> {learning.isPending ? 'Запускаем…' : 'Начать занятие'}</button><button className="button secondary" onClick={onEdit}><Icon name="edit" /> Изменить</button><button className="button secondary" onClick={onAddChild}><Icon name="plus" /> Подтема</button><button className="button secondary" onClick={() => setDocumentOpen(true)}><Icon name="note" /> Конспект</button><details className="order-menu"><summary className="button tertiary">Порядок в структуре</summary><div><button className="button tertiary" disabled={siblingIndex <= 0} onClick={() => onMove(-1)}>Переместить выше в списке</button><button className="button tertiary" disabled={siblingIndex < 0 || siblingIndex >= siblings.length - 1} onClick={() => onMove(1)}>Переместить ниже в списке</button></div></details><button className="button danger ghost-danger" onClick={onDelete}><Icon name="trash" /> Удалить</button></div><ErrorMessage error={learning.error} /><TopicChecklist topicId={topic.id} /><form className="notes-form" onSubmit={(event) => { event.preventDefault(); note.mutate(noteValue) }}><label htmlFor="topic-note">Заметка</label>{noteQuery.isPending && <p className="field-hint" role="status">Загрузка…</p>}<textarea id="topic-note" value={noteValue} onChange={(event) => setNoteDraft(event.target.value)} rows={8} maxLength={10000} placeholder="Зафиксируйте ключевые мысли, ссылки или вопросы…" /><ErrorMessage error={noteQuery.error} /><ErrorMessage error={note.error} /><button className="button primary" disabled={note.isPending || noteQuery.isPending}>{note.isPending ? 'Сохраняем…' : 'Сохранить заметку'}</button></form>{documentOpen && <Suspense fallback={null}><TopicDocumentDialog topicId={topic.id} onClose={() => setDocumentOpen(false)} /></Suspense>}</div>
}

function TopicForm({ roadmapId, topics, initial, onClose, onSaved }: { roadmapId: string; topics: Topic[]; initial: { topic?: Topic; parentId?: string | null }; onClose: () => void; onSaved: (topic: Topic) => void }) {
  const topic = initial.topic
  const [title, setTitle] = useState(topic?.title ?? '')
  const [description, setDescription] = useState(topic?.description ?? '')
  const [kind, setKind] = useState<'topic' | 'task'>(topic?.kind ?? 'topic')
  const [taskStatus, setTaskStatus] = useState(topic?.task_status ?? 'not_started')
  const [priority, setPriority] = useState<TopicPriority>(topic?.priority ?? 'important')
  const priorityField = <label>Приоритет<select value={priority} onChange={(event) => setPriority(event.target.value as TopicPriority)}><option value="critical">Критично</option><option value="important">Важно</option><option value="optional">Дополнительно</option><option value="skippable">Можно пропустить</option></select></label>
  const [parentId, setParentId] = useState<string>(topic?.parent_id ?? initial.parentId ?? '')
  const forbidden = topic ? descendantsOf(topic.id, topics) : new Set<string>()
  if (topic) forbidden.add(topic.id)
  const mutation = useMutation({ mutationFn: async () => {
    const input = { title: title.trim(), description: description.trim(), parent_id: parentId || null, sort_order: topic?.sort_order ?? nextOrder(parentId || null, topics), kind, task_status: kind === 'task' ? taskStatus : 'not_started', priority }
    return topic ? api.updateTopic(topic.id, input) : api.createTopic(roadmapId, input)
  }, onSuccess: onSaved })
  return <Dialog title={topic ? 'Изменить тему' : 'Новая тема'} onClose={onClose}>
    <form className="dialog-body form-stack" onSubmit={(event: FormEvent) => { event.preventDefault(); if (title.trim()) mutation.mutate() }}>
      <label>Название<input value={title} onChange={(event) => setTitle(event.target.value)} maxLength={160} required autoFocus /></label>
      {priorityField}
      <label>Тип<select value={kind} onChange={(event) => setKind(event.target.value as 'topic' | 'task')}><option value="topic">Тема</option><option value="task">Задача</option></select></label>
      {kind === 'task' && <label>Статус<select value={taskStatus} onChange={(event) => setTaskStatus(event.target.value as typeof taskStatus)}><option value="not_started">К выполнению</option><option value="in_progress">В работе</option><option value="completed">Готово</option></select></label>}
      <label>Описание<textarea value={description} onChange={(event) => setDescription(event.target.value)} rows={4} maxLength={2000} /></label>
      <label>Раздел<select value={parentId} onChange={(event) => setParentId(event.target.value)}><option value="">Верхний уровень</option>{topics.filter((item) => !forbidden.has(item.id)).map((item) => <option key={item.id} value={item.id}>{topicPath(item, topics)}</option>)}</select></label>
      <ErrorMessage error={mutation.error} />
      <div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose}>Отмена</button><button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? 'Сохраняем…' : 'Сохранить'}</button></div>
    </form>
  </Dialog>
}

function DependenciesDialog({ roadmapId, topics, dependencies, onClose }: { roadmapId: string; topics: Topic[]; dependencies: Dependency[]; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [predecessor, setPredecessor] = useState('')
  const [successor, setSuccessor] = useState('')
  const conflicts = dependencies.filter((edge) => topics.find((item) => item.id === edge.predecessor_topic_id)?.priority === 'skippable' && ['critical', 'important'].includes(topics.find((item) => item.id === edge.successor_topic_id)?.priority ?? ''))
  const conflictMessage = conflicts.length ? 'Конфликт приоритета: элемент «Можно пропустить» является обязательным основанием для важной или критичной темы. Пересмотрите приоритет или порядок.' : ''
  const [localError, setLocalError] = useState(conflictMessage)
  const mutation = useMutation({ mutationFn: (next: Dependency[]) => api.putDependencies(roadmapId, next), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['dependencies', roadmapId] }) } })
  const add = () => {
    if (conflicts.length) { setLocalError('Конфликт приоритета: skippable prerequisite блокирует важную или критичную тему. Пересмотрите приоритет или порядок.'); return }
    if (!predecessor || !successor) { setLocalError('Выберите обе темы.'); return }
    if (wouldCreateDependencyCycle(dependencies, predecessor, successor)) { setLocalError('Такая связь создаст цикл. Выберите другое направление.'); return }
    if (dependencies.some((item) => item.predecessor_topic_id === predecessor && item.successor_topic_id === successor)) { setLocalError('Такая связь уже есть.'); return }
    setLocalError('')
    mutation.mutate([...dependencies, { predecessor_topic_id: predecessor, successor_topic_id: successor }])
  }
  return <Dialog title="Зависимости" onClose={onClose}><div className="dialog-body dependencies-dialog"><div className="dependency-form"><label>Сначала<select value={predecessor} onChange={(event) => setPredecessor(event.target.value)}><option value="">Выберите тему</option>{topics.map((topic) => <option key={topic.id} value={topic.id}>{topicPath(topic, topics)}</option>)}</select></label><span aria-hidden="true">→</span><label>Затем<select value={successor} onChange={(event) => setSuccessor(event.target.value)}><option value="">Выберите тему</option>{topics.map((topic) => <option key={topic.id} value={topic.id}>{topicPath(topic, topics)}</option>)}</select></label><button className="button primary" type="button" onClick={add} disabled={mutation.isPending}>Добавить</button></div>{localError && <p className="form-error" role="alert">{localError}</p>}<ErrorMessage error={mutation.error} /><ul className="dependency-list">{dependencies.length ? dependencies.map((edge) => { const from = topics.find((item) => item.id === edge.predecessor_topic_id); const to = topics.find((item) => item.id === edge.successor_topic_id); return <li key={`${edge.predecessor_topic_id}-${edge.successor_topic_id}`}><span>{from?.title ?? 'Удалённая тема'} <b>→</b> {to?.title ?? 'Удалённая тема'}</span><button className="icon-button danger" aria-label={`Удалить связь ${from?.title ?? ''} — ${to?.title ?? ''}`} disabled={mutation.isPending} onClick={() => mutation.mutate(dependencies.filter((item) => item !== edge))}><Icon name="trash" /></button></li> }) : <li className="muted">Связей пока нет.</li>}</ul><div className="dialog-actions"><button className="button secondary" onClick={onClose}>Готово</button></div></div></Dialog>
}

function EmptyTopics({ onCreate }: { onCreate: () => void }) { return <div className="canvas-empty"><Icon name="map" /><h2>Нет тем</h2><button className="button primary" onClick={onCreate}><Icon name="plus" /> Добавить тему</button></div> }

function defaultPosition(topic: Topic, topics: Topic[], index: number) {
  const depth = topicDepth(topic, topics)
  const siblings = topics.filter((item) => item.parent_id === topic.parent_id).sort((left, right) => left.sort_order - right.sort_order)
  return { x: depth * 300 + 40, y: Math.max(0, siblings.findIndex((item) => item.id === topic.id)) * 170 + (index % 2) * 20 + 40 }
}

function topicDepth(topic: Topic, topics: Topic[]) { let depth = 0; let parent = topic.parent_id; const seen = new Set<string>(); while (parent && !seen.has(parent)) { seen.add(parent); depth += 1; parent = topics.find((item) => item.id === parent)?.parent_id ?? null } return depth }
function nextOrder(parentId: string | null, topics: Topic[]) { return Math.max(-1, ...topics.filter((item) => item.parent_id === parentId).map((item) => item.sort_order)) + 1 }
function priorityRank(priority: Topic['priority']) { return ({ critical: 0, important: 1, optional: 2, skippable: 3 } as const)[priority ?? 'important'] }
function topicPath(topic: Topic, topics: Topic[]) { const trail = [topic.title]; let parent = topic.parent_id; const seen = new Set<string>(); while (parent && !seen.has(parent)) { seen.add(parent); const found = topics.find((item) => item.id === parent); if (!found) break; trail.unshift(found.title); parent = found.parent_id } return trail.join(' / ') }
async function moveTopic(direction: -1 | 1, topic: Topic, topics: Topic[], client: typeof api, invalidate: () => Promise<unknown>) { const siblings = topics.filter((item) => item.parent_id === topic.parent_id).sort((left, right) => left.sort_order - right.sort_order); const index = siblings.findIndex((item) => item.id === topic.id); const other = siblings[index + direction]; if (!other) return; await Promise.all([client.updateTopic(topic.id, { sort_order: other.sort_order }), client.updateTopic(other.id, { sort_order: topic.sort_order })]); await invalidate() }
