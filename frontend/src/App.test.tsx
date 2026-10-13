import { describe, expect, it, afterEach } from 'vitest'
import { readRoute } from './route'

describe('application routes', () => {
  const originalPath = window.location.pathname

  afterEach(() => window.history.replaceState({}, '', originalPath))

  it('reads the learning route', () => {
    window.history.replaceState({}, '', '/learning')
    expect(readRoute()).toEqual({ page: 'learning' })
  })

  it('keeps roadmap ids encoded in the roadmap route', () => {
    window.history.replaceState({}, '', '/roadmaps/intro%20map')
    expect(readRoute()).toEqual({ page: 'roadmap', id: 'intro map' })
  })
})
