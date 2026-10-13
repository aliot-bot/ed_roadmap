import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ThemeToggle } from './ThemeToggle'

afterEach(() => {
  window.localStorage.clear()
  delete document.documentElement.dataset.theme
})

describe('ThemeToggle', () => {
  it('restores the saved theme and switches between night and day', () => {
    window.localStorage.setItem('roadmap-theme', 'dark')
    render(<ThemeToggle />)

    expect(document.documentElement.dataset.theme).toBe('dark')
    const toggle = screen.getByRole('button', { name: 'Включить светлую тему' })

    fireEvent.click(toggle)

    expect(document.documentElement.dataset.theme).toBe('light')
    expect(window.localStorage.getItem('roadmap-theme')).toBe('light')
    expect(screen.getByRole('button', { name: 'Включить тёмную тему' })).toBeInTheDocument()
  })
})
