import { useEffect, useState, type FormEvent } from 'react'

export default function ResetPassword() {
  const [token] = useState(() => {
    const value = new URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
    return value
  })
  useEffect(() => { window.history.replaceState(null, '', window.location.pathname) }, [])
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (password !== confirmation) {
      setError('Пароли не совпадают')
      return
    }
    setBusy(true)
    setError('')
    try {
      const response = await fetch('/api/v1/auth/reset-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password }),
        signal: AbortSignal.timeout(7000),
      })
      if (!response.ok) {
        setError(response.status === 401
          ? 'Ссылка истекла или уже использована. Запросите новое письмо.'
          : 'Не удалось изменить пароль. Попробуйте ещё раз.')
        return
      }
      setPassword('')
      setConfirmation('')
      setDone(true)
    } catch {
      setError('Не удалось подключиться к сервису.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="page">
      <header className="header"><a className="brand" href="/">Highload Counters</a></header>
      <section className="intro">
        <h1>Новый пароль</h1>
        <p className="description">После изменения пароля все предыдущие сессии завершатся.</p>
      </section>
      {done ? <p role="status">Пароль изменён. Теперь можно войти с новым паролем.</p>
        : !token ? <p role="alert">Откройте ссылку из письма для восстановления пароля.</p>
          : <form className="reset-form" onSubmit={submit}>
            <label htmlFor="password">Пароль (8–128 символов)</label>
            <input id="password" type="password" autoComplete="new-password" required minLength={8} maxLength={128}
              value={password} onChange={(event) => setPassword(event.target.value)} />
            <label htmlFor="confirmation">Повторите пароль</label>
            <input id="confirmation" type="password" autoComplete="new-password" required minLength={8} maxLength={128}
              value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />
            {error && <p role="alert" className="status--error">{error}</p>}
            <button disabled={busy}>{busy ? 'Сохраняем…' : 'Сохранить пароль'}</button>
          </form>}
    </main>
  )
}
