// Development uses the Vite proxy. Production hosting must also route /api to Go.
export async function checkReadiness(): Promise<void> {
  const response = await fetch('/api/readyz', {
    signal: AbortSignal.timeout(2000),
    headers: { Accept: 'application/json' },
  })

  if (!response.ok) {
    throw new Error('API или его зависимости недоступны')
  }

  const body: unknown = await response.json()
  if (
    typeof body !== 'object' ||
    body === null ||
    !('status' in body) ||
    body.status !== 'ready'
  ) {
    throw new Error('Неожиданный ответ API')
  }
}
