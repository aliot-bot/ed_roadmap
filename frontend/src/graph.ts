import type { Dependency, Progress, Topic } from './types'

export type NodeSize = { width: number; height: number }
export type GraphPosition = { x: number; y: number }

const DEFAULT_NODE_SIZE: NodeSize = { width: 272, height: 200 }
const HORIZONTAL_GAP = 76
const VERTICAL_GAP = 44
const OUTER_GAP = 40

/**
 * Produces a deterministic left-to-right layout for the currently visible graph.
 * Both structural links and prerequisites participate in layer calculation. A
 * malformed cycle is broken deterministically, so imported data cannot make the
 * map unusable. After assigning layers, barycentric sweeps order nodes close
 * to their neighbours and parents are centred between their descendants. This
 * avoids the long, top-aligned columns produced by a simple layer-by-layer list.
 */
export function autoLayoutTopics(topics: Topic[], dependencies: Dependency[], sizes: Map<string, NodeSize> = new Map()): Map<string, GraphPosition> {
  const byId = new Map(topics.map((topic) => [topic.id, topic]))
  const compareTopics = (leftId: string, rightId: string) => {
    const left = byId.get(leftId)!
    const right = byId.get(rightId)!
    return left.sort_order - right.sort_order || left.title.localeCompare(right.title, 'ru') || left.id.localeCompare(right.id)
  }
  const successors = new Map<string, Set<string>>(topics.map((topic) => [topic.id, new Set<string>()]))
  const indegree = new Map<string, number>(topics.map((topic) => [topic.id, 0]))
  let relationCount = 0
  const addEdge = (source: string, target: string) => {
    if (source === target || !byId.has(source) || !byId.has(target)) return
    const targets = successors.get(source)!
    if (!targets.has(target)) {
      targets.add(target)
      indegree.set(target, (indegree.get(target) ?? 0) + 1)
      relationCount += 1
    }
  }

  for (const topic of topics) if (topic.parent_id) addEdge(topic.parent_id, topic.id)
  for (const dependency of dependencies) addEdge(dependency.predecessor_topic_id, dependency.successor_topic_id)

  // Imported batches without parent references or dependencies do not form a
  // tree. Keep them in a compact left-to-right grid instead of a long column.
  if (relationCount === 0) return layoutUnconnectedTopics(topics, sizes, compareTopics)

  const rank = new Map<string, number>(topics.map((topic) => [topic.id, 0]))
  const pending = new Set(topics.map((topic) => topic.id))
  const ready = topics.filter((topic) => indegree.get(topic.id) === 0).map((topic) => topic.id).sort(compareTopics)
  const takeNext = () => {
    if (ready.length) return ready.shift()!
    // A dependency imported outside the UI may contain a cycle.  Breaking it at
    // the stable first item gives a useful layout and preserves determinism.
    return [...pending].sort(compareTopics)[0]!
  }

  while (pending.size) {
    const id = takeNext()
    if (!pending.delete(id)) continue
    for (const target of successors.get(id) ?? []) {
      rank.set(target, Math.max(rank.get(target) ?? 0, (rank.get(id) ?? 0) + 1))
      indegree.set(target, (indegree.get(target) ?? 1) - 1)
      if (pending.has(target) && indegree.get(target) === 0) {
        ready.push(target)
        ready.sort(compareTopics)
      }
    }
  }

  const layers = new Map<number, string[]>()
  for (const topic of topics) {
    const layer = rank.get(topic.id) ?? 0
    const entries = layers.get(layer) ?? []
    entries.push(topic.id)
    layers.set(layer, entries)
  }

  const layerNumbers = [...layers.keys()].sort((a, b) => a - b)
  const predecessors = new Map<string, Set<string>>(topics.map((topic) => [topic.id, new Set<string>()]))
  for (const [source, targets] of successors) {
    for (const target of targets) predecessors.get(target)?.add(source)
  }

  // Sort adjacent layers by their connected neighbours. Repeating the downward
  // and upward passes greatly reduces crossings while retaining a stable order
  // for unrelated items.
  for (const layer of layerNumbers) layers.get(layer)!.sort(compareTopics)
  for (let pass = 0; pass < 4; pass += 1) {
    orderLayers(layerNumbers.slice(1), layers, predecessors, compareTopics)
    orderLayers(layerNumbers.slice(0, -1).reverse(), layers, successors, compareTopics)
  }

  const positions = new Map<string, GraphPosition>()
  let x = OUTER_GAP
  for (const layer of layerNumbers) {
    const entries = layers.get(layer)!
    let y = OUTER_GAP
    let widest = DEFAULT_NODE_SIZE.width
    for (const id of entries) {
      const size = sizes.get(id) ?? DEFAULT_NODE_SIZE
      positions.set(id, { x: Math.round(x), y: Math.round(y) })
      y += Math.max(size.height, DEFAULT_NODE_SIZE.height) + VERTICAL_GAP
      widest = Math.max(widest, size.width)
    }
    x += widest + HORIZONTAL_GAP
  }

  // Process from leaves to roots. Moving only the parent side of a connection
  // keeps each branch compact and prevents the whole graph from drifting down
  // on every optimisation pass.
  centreLayers(layerNumbers.slice(0, -1).reverse(), layers, successors, positions, sizes)
  return positions
}

function orderLayers(
  layerNumbers: number[],
  layers: Map<number, string[]>,
  neighbours: Map<string, Set<string>>,
  compareTopics: (leftId: string, rightId: string) => number,
) {
  const order = new Map<string, number>()
  for (const entries of layers.values()) entries.forEach((id, index) => order.set(id, index))
  for (const layer of layerNumbers) {
    const entries = layers.get(layer)!
    entries.sort((left, right) => {
      const leftAverage = neighbourAverage(neighbours.get(left), order)
      const rightAverage = neighbourAverage(neighbours.get(right), order)
      if (leftAverage === undefined && rightAverage === undefined) return compareTopics(left, right)
      if (leftAverage === undefined) return 1
      if (rightAverage === undefined) return -1
      return leftAverage - rightAverage || compareTopics(left, right)
    })
    entries.forEach((id, index) => order.set(id, index))
  }
}

function neighbourAverage(neighbours: Set<string> | undefined, order: Map<string, number>) {
  const indexes = [...(neighbours ?? [])].map((id) => order.get(id)).filter((index): index is number => index !== undefined)
  return indexes.length ? indexes.reduce((sum, index) => sum + index, 0) / indexes.length : undefined
}

function centreLayers(
  layerNumbers: number[],
  layers: Map<number, string[]>,
  neighbours: Map<string, Set<string>>,
  positions: Map<string, GraphPosition>,
  sizes: Map<string, NodeSize>,
) {
  for (const layer of layerNumbers) {
    const entries = layers.get(layer)!
    const centres = entries.map((id) => {
      const nearby = [...(neighbours.get(id) ?? [])]
        .map((neighbour) => positionCentre(neighbour, positions, sizes))
        .filter((centre): centre is number => centre !== undefined)
      return nearby.length ? nearby.reduce((sum, centre) => sum + centre, 0) / nearby.length : positionCentre(id, positions, sizes)!
    })
    placeLayer(entries, centres, positions, sizes)
  }
}

function positionCentre(id: string, positions: Map<string, GraphPosition>, sizes: Map<string, NodeSize>) {
  const position = positions.get(id)
  if (!position) return undefined
  return position.y + (sizes.get(id)?.height ?? DEFAULT_NODE_SIZE.height) / 2
}

function placeLayer(entries: string[], desiredCentres: number[], positions: Map<string, GraphPosition>, sizes: Map<string, NodeSize>) {
  const centres = [...desiredCentres]
  const halfHeight = (id: string) => (sizes.get(id)?.height ?? DEFAULT_NODE_SIZE.height) / 2
  for (let index = 1; index < entries.length; index += 1) {
    const minimum = centres[index - 1]! + halfHeight(entries[index - 1]!) + VERTICAL_GAP + halfHeight(entries[index]!)
    centres[index] = Math.max(centres[index]!, minimum)
  }
  for (let index = entries.length - 2; index >= 0; index -= 1) {
    const maximum = centres[index + 1]! - halfHeight(entries[index + 1]!) - VERTICAL_GAP - halfHeight(entries[index]!)
    centres[index] = Math.min(centres[index]!, maximum)
  }
  const top = Math.min(...centres.map((centre, index) => centre - halfHeight(entries[index]!)))
  const shift = top < OUTER_GAP ? OUTER_GAP - top : 0
  entries.forEach((id, index) => {
    const position = positions.get(id)!
    position.y = Math.round(centres[index]! + shift - halfHeight(id))
  })
}

function layoutUnconnectedTopics(topics: Topic[], sizes: Map<string, NodeSize>, compareTopics: (leftId: string, rightId: string) => number): Map<string, GraphPosition> {
  const ids = topics.map((topic) => topic.id).sort(compareTopics)
  const columns = Math.ceil(Math.sqrt(ids.length))
  const rows = Math.ceil(ids.length / columns)
  const widthByColumn = Array.from({ length: columns }, () => DEFAULT_NODE_SIZE.width)
  const heightByRow = Array.from({ length: rows }, () => DEFAULT_NODE_SIZE.height)

  ids.forEach((id, index) => {
    const size = sizes.get(id) ?? DEFAULT_NODE_SIZE
    const column = index % columns
    const row = Math.floor(index / columns)
    widthByColumn[column] = Math.max(widthByColumn[column]!, size.width)
    heightByRow[row] = Math.max(heightByRow[row]!, size.height)
  })

  const xByColumn: number[] = []
  let x = OUTER_GAP
  widthByColumn.forEach((width) => {
    xByColumn.push(x)
    x += width + HORIZONTAL_GAP
  })
  const yByRow: number[] = []
  let y = OUTER_GAP
  heightByRow.forEach((height) => {
    yByRow.push(y)
    y += height + VERTICAL_GAP
  })

  return new Map(ids.map((id, index) => [id, {
    x: Math.round(xByColumn[index % columns]!),
    y: Math.round(yByRow[Math.floor(index / columns)]!),
  }]))
}

export function childTopics(topics: Topic[], parentId: string): Topic[] {
  return topics.filter((topic) => topic.parent_id === parentId)
}

export function topicDepth(topic: Topic, topics: Topic[]) {
  let depth = 1
  let parentId = topic.parent_id
  const seen = new Set<string>([topic.id])
  while (parentId && !seen.has(parentId)) {
    seen.add(parentId)
    depth += 1
    parentId = topics.find((candidate) => candidate.id === parentId)?.parent_id ?? null
  }
  return depth
}

export function collapsedAtLevel(topics: Topic[], level: number) {
  return new Set(topics.filter((topic) => topicDepth(topic, topics) >= level && childTopics(topics, topic.id).length > 0).map((topic) => topic.id))
}

export function hiddenTopicIds(topics: Topic[], collapsed: Set<string>): Set<string> {
  const hidden = new Set<string>()
  const hideChildren = (parentId: string) => {
    for (const child of childTopics(topics, parentId)) {
      hidden.add(child.id)
      hideChildren(child.id)
    }
  }
  for (const id of collapsed) hideChildren(id)
  return hidden
}

export function topicProgress(topicId: string, topics: Topic[]): Progress {
  const descendants: Topic[] = []
  const visit = (id: string) => {
    const children = childTopics(topics, id)
    if (children.length === 0) {
      const topic = topics.find((item) => item.id === id)
      if (topic && topic.kind !== 'task') descendants.push(topic)
      return
    }
    children.forEach((child) => visit(child.id))
  }
  visit(topicId)
  const completed = descendants.filter((topic) => topic.completed).length
  return { completed, total: descendants.length, percent: descendants.length ? Math.round((completed / descendants.length) * 100) : null }
}

export function descendantsOf(topicId: string, topics: Topic[]): Set<string> {
  const result = new Set<string>()
  const visit = (id: string) => childTopics(topics, id).forEach((child) => { result.add(child.id); visit(child.id) })
  visit(topicId)
  return result
}

export function wouldCreateDependencyCycle(dependencies: Dependency[], predecessor: string, successor: string): boolean {
  if (predecessor === successor) return true
  const adjacency = new Map<string, string[]>()
  for (const edge of dependencies) {
    const targets = adjacency.get(edge.predecessor_topic_id) ?? []
    targets.push(edge.successor_topic_id)
    adjacency.set(edge.predecessor_topic_id, targets)
  }
  const seen = new Set<string>()
  const visit = (id: string): boolean => {
    if (id === predecessor) return true
    if (seen.has(id)) return false
    seen.add(id)
    return (adjacency.get(id) ?? []).some(visit)
  }
  return visit(successor)
}
