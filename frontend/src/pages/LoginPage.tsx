import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api } from '../api'
import { Icon } from '../components/Icon'
import { ThemeToggle } from '../components/ThemeToggle'

export function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [key, setKey] = useState('')
  const [showKey, setShowKey] = useState(false)
  const mutation = useMutation({
    mutationFn: () => api.keyLogin(key.trim()),
    onSuccess,
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (key.trim()) mutation.mutate()
  }

  return (
    <main className="login-page">
      <div className="login-theme"><ThemeToggle /></div>
      <section className="login-intro" aria-label="О приложении">
        <span className="brand-mark large" aria-hidden="true"><Icon name="map" /></span>
        <h1>Карты знаний</h1>
      </section>
      <section className="login-panel">
        <form className="auth-card" onSubmit={submit}>
          <p className="eyebrow">Личное пространство</p>
          <h2>Вход по ключу</h2>
          <label>
            Ключ доступа
            <span className="password-field"><input type={showKey ? 'text' : 'password'} autoComplete="off" value={key} onChange={(event) => setKey(event.target.value)} required autoFocus /><button type="button" className="password-toggle" onClick={() => setShowKey((value) => !value)} aria-label={showKey ? 'Скрыть ключ' : 'Показать ключ'}>{showKey ? 'Скрыть' : 'Показать'}</button></span>
          </label>
          {mutation.isError && <p className="form-error" role="alert">Неверный ключ</p>}
          <button className="button primary wide" disabled={mutation.isPending}>{mutation.isPending ? 'Проверяем…' : 'Войти'}</button>
        </form>
      </section>
    </main>
  )
}
