import { useState } from 'react'
import { checkReadiness } from './lib/api'

type Status = 'idle' | 'loading' | 'ready' | 'error'

const messages: Record<Status, string> = {
  idle: 'Подключение ещё не проверено',
  loading: 'Проверяем API, PostgreSQL и Redis…',
  ready: 'API, PostgreSQL и Redis доступны',
  error: 'Нет подключения к API или его зависимостям',
}

export default function App() {
  const [status, setStatus] = useState<Status>('idle')

  async function checkConnection() {
    setStatus('loading')
    try {
      await checkReadiness()
      setStatus('ready')
    } catch {
      setStatus('error')
    }
  }

  return (
    <main className="page">
      <header className="header">
        <a className="brand" href="/">Highload Counters</a>
        <span className="badge">SIS2 · T7</span>
      </header>

      <section className="intro">
        <p className="eyebrow">Лайки и просмотры</p>
        <h1>Каждое действие<br />имеет значение.</h1>
        <p className="description">
          Учебный проект для изучения счётчиков, идемпотентности
          и сравнения REST с gRPC.
        </p>
      </section>

      <section className="panel" aria-labelledby="connection-title">
        <div>
          <h2 id="connection-title">Подключение к сервису</h2>
          <p role="status" className={`status status--${status}`}>
            {messages[status]}
          </p>
        </div>
        <button disabled={status === 'loading'} onClick={checkConnection}>
          {status === 'loading' ? 'Проверяем…' : 'Проверить подключение'}
        </button>
      </section>

      <section className="placeholder" aria-labelledby="posts-title">
        <span className="placeholder-icon" aria-hidden="true">＋</span>
        <h2 id="posts-title">Здесь появятся тестовые посты</h2>
        <p>Лайки, просмотры и счётчики подключим после реализации API.</p>
      </section>
      <footer>React + TypeScript · REST API</footer>
    </main>
  )
}
