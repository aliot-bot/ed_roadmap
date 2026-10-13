import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { LearningPage } from './LearningPage'

vi.mock('../components/Dashboard', () => ({
  Dashboard: ({ login }: { login?: string }) => <section aria-label="Обзор обучения">Обзор обучения для {login}</section>,
}))

describe('LearningPage', () => {
  it('renders the learning overview dashboard', () => {
    render(<LearningPage login="Dior" />)
    expect(screen.getByRole('region', { name: 'Обзор обучения' })).toHaveTextContent('Dior')
  })
})
