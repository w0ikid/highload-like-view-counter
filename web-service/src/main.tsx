import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import ResetPassword from './ResetPassword'
import './styles.css'

const isResetPage = window.location.pathname === '/reset-password'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {isResetPage ? <ResetPassword /> : <App />}
  </StrictMode>,
)
