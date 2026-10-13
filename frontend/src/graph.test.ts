import { describe, expect, it } from 'vitest'
import { autoLayoutTopics, hiddenTopicIds, topicProgress, wouldCreateDependencyCycle } from './graph'
import type { Topic } from './types'

const topics: Topic[] = [
  { id: 'root', roadmap_id: 'r', parent_id: null, title: 'Root', description: '', sort_order: 0, completed: false },
  { id: 'one', roadmap_id: 'r', parent_id: 'root', title: 'One', description: '', sort_order: 0, completed: true },
  { id: 'two', roadmap_id: 'r', parent_id: 'root', title: 'Two', description: '', sort_order: 1, completed: false },
]

describe('graph helpers', () => {
  it('counts only leaf topics for group progress', () => expect(topicProgress('root', topics)).toEqual({ completed: 1, total: 2, percent: 50 }))
  it('hides nested descendants of a collapsed topic', () => expect([...hiddenTopicIds(topics, new Set(['root']))].sort()).toEqual(['one', 'two']))
  it('prevents self references and directed cycles', () => {
    const dependencies = [{ predecessor_topic_id: 'one', successor_topic_id: 'two' }]
    expect(wouldCreateDependencyCycle(dependencies, 'two', 'one')).toBe(true)
    expect(wouldCreateDependencyCycle(dependencies, 'one', 'one')).toBe(true)
    expect(wouldCreateDependencyCycle(dependencies, 'root', 'one')).toBe(false)
  })

  it('keeps graph visibility correct for a long collapsed chain', () => {
    const chain = makeChain(700)
    const collapsedAt = 237
    const hidden = hiddenTopicIds(chain, new Set([`topic-${collapsedAt}`]))

    expect(hidden.size).toBe(chain.length - collapsedAt - 1)
    expect(hidden.has(`topic-${collapsedAt}`)).toBe(false)
    expect(hidden.has(`topic-${collapsedAt + 1}`)).toBe(true)
    expect(hidden.has('topic-699')).toBe(true)
    expect([...hiddenTopicIds(chain, new Set([`topic-${collapsedAt}`]))]).toEqual([...hidden])
  })

  it('counts leaves, not completed internal nodes, in a deep mixed tree', () => {
    const root: Topic = topic('mixed-root', null, true)
    const groupA: Topic = topic('group-a', root.id, true)
    const groupB: Topic = topic('group-b', root.id, true)
    const groupC: Topic = topic('group-c', root.id, true)
    const mixed = [root, groupA, groupB, groupC]
    for (let index = 0; index < 100; index += 1) mixed.push(topic(`a-${index}`, groupA.id, index % 2 === 0))
    for (let index = 0; index < 80; index += 1) mixed.push(topic(`b-${index}`, groupB.id, index % 3 === 0))
    let parent = groupC.id
    for (let index = 0; index < 120; index += 1) {
      const node = topic(`c-${index}`, parent, true)
      mixed.push(node)
      parent = node.id
    }
    mixed.push(topic('c-leaf', parent, true))

    expect(topicProgress(root.id, mixed)).toEqual({ completed: 78, total: 181, percent: 43 })
    expect(topicProgress(groupC.id, mixed)).toEqual({ completed: 1, total: 1, percent: 100 })
  })

  it('detects a cycle added at the end of a long DAG', () => {
    const count = 1100
    const dependencies = Array.from({ length: count - 1 }, (_, index) => ({
      predecessor_topic_id: `topic-${index}`,
      successor_topic_id: `topic-${index + 1}`,
    }))

    expect(wouldCreateDependencyCycle(dependencies, 'topic-1099', 'topic-366')).toBe(true)
    expect(wouldCreateDependencyCycle(dependencies, 'external', 'topic-366')).toBe(false)
    expect(wouldCreateDependencyCycle(dependencies, 'topic-550', 'unrelated')).toBe(false)
  })

  it('lays out a forest in deterministic dependency-aware layers without overlaps', () => {
    const forest = [
      topic('a-root', null, false),
      topic('a-child', 'a-root', false),
      topic('b-root', null, false),
      topic('b-child', 'b-root', false),
      topic('isolated', null, false),
    ]
    forest[3]!.sort_order = 1
    const dependencies = [{ predecessor_topic_id: 'a-child', successor_topic_id: 'b-child' }]
    const sizes = new Map(forest.map((item) => [item.id, { width: 252, height: 160 }]))
    const first = autoLayoutTopics(forest, dependencies, sizes)
    const second = autoLayoutTopics(forest, dependencies, sizes)

    expect(first).toEqual(second)
    expect(first.get('a-root')!.x).toBeLessThan(first.get('a-child')!.x)
    expect(first.get('a-child')!.x).toBeLessThan(first.get('b-child')!.x)
    expect(noOverlaps(first, sizes)).toBe(true)
  })

  it('spreads an unconnected imported batch into a compact grid instead of one vertical column', () => {
    const imported = Array.from({ length: 12 }, (_, index) => topic(`import-${index}`, null, false))
    const sizes = new Map(imported.map((item) => [item.id, { width: 252, height: 160 }]))
    const positions = autoLayoutTopics(imported, [], sizes)

    expect(new Set([...positions.values()].map((position) => position.x)).size).toBeGreaterThan(1)
    expect(new Set([...positions.values()].map((position) => position.y)).size).toBeGreaterThan(1)
    expect(noOverlaps(positions, sizes)).toBe(true)
  })

  it('centres a parent over its branch instead of pinning it to the top of its layer', () => {
    const tree = [
      topic('root', null, false),
      topic('alpha', 'root', false),
      topic('beta', 'root', false),
      topic('alpha-leaf', 'alpha', false),
      topic('beta-leaf', 'beta', false),
    ]
    const sizes = new Map(tree.map((item) => [item.id, { width: 252, height: 160 }]))
    const positions = autoLayoutTopics(tree, [], sizes)
    const centre = (id: string) => positions.get(id)!.y + 80

    expect(centre('root')).toBe((centre('alpha') + centre('beta')) / 2)
    expect(positions.get('root')!.y).toBeGreaterThan(40)
    expect(noOverlaps(positions, sizes)).toBe(true)
  })

  it('keeps producing usable positions for malformed dependency cycles and unknown endpoints', () => {
    const cycle = [topic('a', null, false), topic('b', null, false), topic('c', null, false)]
    const positions = autoLayoutTopics(cycle, [
      { predecessor_topic_id: 'a', successor_topic_id: 'b' },
      { predecessor_topic_id: 'b', successor_topic_id: 'a' },
      { predecessor_topic_id: 'missing', successor_topic_id: 'c' },
    ])

    expect([...positions.keys()].sort()).toEqual(['a', 'b', 'c'])
    expect(noOverlaps(positions, new Map())).toBe(true)
  })
})

function noOverlaps(positions: Map<string, { x: number; y: number }>, sizes: Map<string, { width: number; height: number }>) {
  const items = [...positions.entries()]
  return items.every(([id, position], index) => items.slice(index + 1).every(([otherId, other]) => {
    const first = sizes.get(id) ?? { width: 272, height: 200 }
    const second = sizes.get(otherId) ?? { width: 272, height: 200 }
    return position.x + first.width <= other.x || other.x + second.width <= position.x || position.y + first.height <= other.y || other.y + second.height <= position.y
  }))
}

function topic(id: string, parentId: string | null, completed: boolean): Topic {
  return { id, roadmap_id: 'roadmap', parent_id: parentId, title: id, description: '', sort_order: 0, completed }
}

function makeChain(length: number): Topic[] {
  return Array.from({ length }, (_, index) => topic(`topic-${index}`, index === 0 ? null : `topic-${index - 1}`, false))
}
