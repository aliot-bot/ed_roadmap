export type Route = { page: 'roadmaps' } | { page: 'learning' } | { page: 'roadmap'; id: string }

export function readRoute(): Route {
  if (window.location.pathname === '/learning') return { page: 'learning' }
  const match = window.location.pathname.match(/^\/roadmaps\/([^/]+)$/)
  return match?.[1] ? { page: 'roadmap', id: decodeURIComponent(match[1]) } : { page: 'roadmaps' }
}
