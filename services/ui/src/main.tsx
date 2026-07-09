import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import './index.css'
import { ThemeProvider } from '@/components/theme-provider'
import App from '@/App.tsx'
import { ImportSettings } from '@/pages/import-settings'
import { loadSettings } from '@/lib/settings'

// In-memory routing (the URL never changes): on startup show the settings
// import page unless a valid settings file was already imported (persisted in
// localStorage).
const router = createMemoryRouter(
  [
    { path: '/', element: <App /> },
    { path: '/import', element: <ImportSettings /> },
  ],
  { initialEntries: [loadSettings() ? '/' : '/import'] },
)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <RouterProvider router={router} />
    </ThemeProvider>
  </StrictMode>,
)
